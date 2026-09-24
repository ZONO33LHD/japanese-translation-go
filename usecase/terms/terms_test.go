package terms

import (
	"index/suffixarray"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/sudachi"
	"github.com/ZONO33LHD/japanese-translation-go/internal/testenv"
)

// charClassTokenizer は文字種の連続で区切り、カタカナ・英字の連続を名詞、それ以外を 1 文字ずつ助詞とする。
// properNouns に含まれる表層は固有名詞にする。
type charClassTokenizer struct {
	properNouns map[string]bool
}

func class(r rune) int {
	switch {
	case unicode.In(r, unicode.Katakana) || r == 'ー':
		return 1
	case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
		return 2
	case unicode.Is(unicode.Han, r):
		return 3
	}
	return 0
}

func (c charClassTokenizer) Tokenize(text string) ([]model.Morpheme, error) {
	rs := []rune(text)
	var out []model.Morpheme
	for i := 0; i < len(rs); {
		cl := class(rs[i])
		j := i + 1
		if cl != 0 {
			for j < len(rs) && class(rs[j]) == cl {
				j++
			}
		}
		s := string(rs[i:j])
		pos := [6]string{"助詞", "*", "*", "*", "*", "*"}
		switch {
		case c.properNouns[s]:
			pos[0], pos[1] = "名詞", "固有名詞"
		case cl != 0:
			pos[0], pos[1] = "名詞", "普通名詞"
		}
		out = append(out, model.Morpheme{Surface: s, POS: pos, Begin: i, End: j})
		i = j
	}
	return out, nil
}

func TestASCIIAcronyms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"APIとは", []string{"API"}},
		{"HTTP2とTLS", []string{"HTTP2", "TLS"}},
		{"AbcやABcやxAPI", nil},
		{"A と B", nil},
		{"(AWS)", []string{"AWS"}},
	}
	for _, c := range cases {
		rs := []rune(c.in)
		var got []string
		for _, s := range asciiAcronyms(rs) {
			got = append(got, string(rs[s[0]:s[1]]))
		}
		if len(got) != len(c.want) {
			t.Errorf("asciiAcronyms(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("asciiAcronyms(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	}
}

func TestBuildInventory(t *testing.T) {
	text := "# Cloudflare入門\n\n" +
		"Cloudflareのワーカーは速い。APIとは窓口のことだ。\n" +
		"<!-- メモ: ワーカー -->\n" +
		"ワーカーを使う。KV（キーバリュー）も使う。"
	tk := charClassTokenizer{properNouns: map[string]bool{}}
	got, err := BuildInventory(text, tk)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		term  string
		line  int
		count int
		hint  bool
	}{
		{"Cloudflare", 1, 2, true},
		{"ワーカー", 3, 2, true},
		{"API", 3, 1, true},
		{"KV", 5, 1, true},
		{"キーバリュー", 5, 1, true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		g := got[i]
		if g.Term != w.term || g.FirstLine != w.line || g.Count != w.count || g.HasGlossHint != w.hint {
			t.Errorf("term %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestContextGlossHintByParenthesis(t *testing.T) {
	text := "本文でSLO（サービスレベル目標）を定める。"
	got, err := BuildInventory(text, charClassTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Term != "SLO" || !got[0].HasGlossHint || got[0].Context != text {
		t.Errorf("got %+v", got)
	}
}

func TestFixturesWithRealTokenizer(t *testing.T) {
	path := testenv.RequireDict(t)
	tk, err := sudachi.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tk.Close() })
	files, _ := filepath.Glob("../../testdata/fixtures/*.md")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ts, err := BuildInventory(string(b), tk)
		if err != nil {
			t.Fatal(err)
		}
		prev := 0
		for _, term := range ts {
			if term.FirstLine < prev {
				t.Errorf("%s: terms not in first-appearance order: %+v", f, ts)
			}
			prev = term.FirstLine
			if term.Count < 1 {
				t.Errorf("%s: %q count = %d", f, term.Term, term.Count)
			}
		}
	}
}

func TestCountNonOverlappingMatchesStringsCount(t *testing.T) {
	text := "ーーーー AIとAI、AAA。データデータ"
	index := suffixarray.New([]byte(text))
	for _, term := range []string{"ーー", "AI", "AA", "データ", "無い"} {
		if got, want := countNonOverlapping(index, term), strings.Count(text, term); got != want {
			t.Errorf("count(%q) = %d, want %d", term, got, want)
		}
	}
}

// 文書全体が 1 行で用語が多いとき、用語ごとに行を先頭から探し直すと用語数×文書長になる。
// 旧実装は 8,000 語で数秒かかっていた。
func TestBuildInventoryLinearOnLongLineWithManyTerms(t *testing.T) {
	var b strings.Builder
	kana := []rune("アイウエオカキクケコサシスセソタチツテト")
	for i := range 8000 {
		b.WriteString(string([]rune{kana[i%20], kana[(i/20)%20], kana[(i/400)%20], 'ー'}))
		b.WriteString("と")
	}
	start := time.Now()
	terms, err := BuildInventory(b.String(), charClassTokenizer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 8000 {
		t.Fatalf("got %d terms", len(terms))
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}
