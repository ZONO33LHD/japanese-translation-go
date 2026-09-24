package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/sudachi"
	"github.com/ZONO33LHD/japanese-translation-go/internal/testenv"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
)

func openTokenizer(t *testing.T) *sudachi.Tokenizer {
	t.Helper()
	path := testenv.RequireDict(t)
	tk, err := sudachi.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tk.Close() })
	return tk
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// 元実装の dev/check-fixtures.sh が固定している検出件数と一致することを確かめる回帰テスト。
// 検出器や fixture を意図して変えた場合はここの期待値を更新する。
func TestFixtureFindingCounts(t *testing.T) {
	tk := openTokenizer(t)
	svc := lint.NewService(tk)
	cases := []struct {
		fixture      string
		experimental bool
		want         int
	}{
		{"ai-smelly.md", false, 25},
		{"ai-smelly.md", true, 33},
		{"natural.md", false, 0},
		{"natural.md", true, 0},
	}
	for _, c := range cases {
		findings, stats, err := svc.Run(readFixture(t, c.fixture), lint.Options{Experimental: c.experimental})
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != c.want || stats.TotalFindings != c.want {
			t.Errorf("%s (experimental=%v): got %d findings, want %d", c.fixture, c.experimental, len(findings), c.want)
		}
	}
}

// 元実装の出力（ai-smelly.md, --json）から採った集計値。
func TestFixtureStatsMatchOriginal(t *testing.T) {
	tk := openTokenizer(t)
	_, stats, err := lint.NewService(tk).Run(readFixture(t, "ai-smelly.md"), lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	wantByCategory := []lint.CategoryCount{
		{"forbidden_phrase", 8},
		{"antithesis_repetition", 5},
		{"low_burstiness", 1},
		{"english_syntax_inanimate_subject", 2},
		{"inanimate_subject_morph", 1},
		{"low_specificity", 2},
		{"translationese", 5},
		{"translationese_morph", 1},
	}
	if len(stats.ByCategory) != len(wantByCategory) {
		t.Fatalf("by_category = %v, want %v", stats.ByCategory, wantByCategory)
	}
	for i, want := range wantByCategory {
		if stats.ByCategory[i] != want {
			t.Errorf("by_category[%d] = %v, want %v", i, stats.ByCategory[i], want)
		}
	}
	m := stats.Morph
	if m.TotalSentences != 29 || m.NominalEndingCount != 6 || m.TotalParagraphs != 11 || m.ParagraphLeadConjunctionCount != 4 {
		t.Errorf("morph stats = %+v", m)
	}
	if m.NominalEndingRatio != 0.20689655172413793 || m.ParagraphLeadConjunctionRatio != 0.36363636363636365 {
		t.Errorf("ratios = %v, %v", m.NominalEndingRatio, m.ParagraphLeadConjunctionRatio)
	}
}

func TestReadingLoadRunsIndependently(t *testing.T) {
	tk := openTokenizer(t)
	svc := lint.NewService(tk)
	text := readFixture(t, "ai-smelly.md")
	doc, err := svc.Prepare(text)
	if err != nil {
		t.Fatal(err)
	}
	findings, stats := lint.ReadingLoad(doc, model.GenreNone)
	if stats.Total != len(findings) || stats.Sentences != 29 {
		t.Errorf("reading load stats = %+v (findings=%d)", stats, len(findings))
	}
	for _, f := range findings {
		if !lint.ReadingLoadCategories[f.Category] {
			t.Errorf("unexpected category in reading-load lane: %s", f.Category)
		}
	}
}

func TestBusinessGenreDisablesStructuralCategories(t *testing.T) {
	tk := openTokenizer(t)
	findings, _, err := lint.NewService(tk).Run(readFixture(t, "ai-smelly.md"),
		lint.Options{Genre: model.GenreBusiness, Experimental: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		switch f.Category {
		case "high_bullet_ratio", "high_bold_density", "boilerplate_heading", "numbered_phase_structure":
			t.Errorf("business genre must drop %s even with --experimental", f.Category)
		}
	}
}

// 文頭のインラインコードはマスクで空白になり strip で落ちる。形態素のオフセットを原文に
// そのまま当てると excerpt が「`kubectl 」のように壊れていた回帰を防ぐ。
func TestMorphExcerptsAlignWithLeadingInlineCode(t *testing.T) {
	tk := openTokenizer(t)
	text := "`kubectl apply`を使うことができる。\n\n`config.yaml`これは状況の変化を示す。\n\n" +
		"まず`kubectl apply`をすることができる。\n\n[リンク](http://example.com)それは状況の変化をもたらす。\n"
	findings, _, err := lint.NewService(tk).Run(text, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, f := range findings {
		if f.Category == "translationese_morph" || f.Category == "inanimate_subject_morph" {
			got[f.Line] = f.Excerpt
		}
	}
	// 空白化された区間の途中から切り出した断片（「y`を」「om)それは」）が出ないこと。
	for line, excerpt := range got {
		if strings.ContainsAny(excerpt, "`abcdefghijklmnopqrstuvwxyz") {
			t.Errorf("L%d excerpt contains a fragment of a masked span: %q", line, excerpt)
		}
	}
	if got[1] != "を使うことができる" || got[3] != "これは状況の変化を示す" || len(got) != 4 {
		t.Errorf("excerpts = %v", got)
	}
}

// 行は LF だけで区切る。U+2028 で区切ると outline やエディタと行番号がずれる。
func TestLineNumbersIgnoreUnicodeLineSeparator(t *testing.T) {
	tk := openTokenizer(t)
	text := "前半 後半です。\n\n重要なのは中身です。\n"
	findings, _, err := lint.NewService(tk).Run(text, lint.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 || findings[0].Line != 3 {
		t.Fatalf("findings = %+v, want forbidden_phrase at L3", findings)
	}
}

// 句点の多い長い 1 行でも線形時間で終わること（以前はマッチごとに先頭から数え直して O(n²) だった）。
func TestLongSingleLineFinishesQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("long input")
	}
	tk := openTokenizer(t)
	var b []byte
	for range 20000 {
		b = append(b, "あ。"...)
	}
	start := time.Now()
	if _, _, err := lint.NewService(tk).Run(string(b), lint.Options{}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("took %v for 20,000 sentences on one line", d)
	}
}
