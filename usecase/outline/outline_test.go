package outline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/sudachi"
	"github.com/ZONO33LHD/japanese-translation-go/internal/testenv"
)

// fakeTokenizer は登録済みの見出しだけを決まった品詞列に分解する。
type fakeTokenizer map[string][]model.Morpheme

func (f fakeTokenizer) Tokenize(text string) ([]model.Morpheme, error) { return f[text], nil }

func m(surface, pos string) model.Morpheme {
	return model.Morpheme{Surface: surface, POS: [6]string{pos, "*", "*", "*", "*", "*"}}
}

func TestBuild(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Entry
	}{
		{
			name: "heading, lead and bullets",
			in:   "# タイトル\n\n最初の文。次の文。\n続き\n\n- a\n- b\n  継続行\n",
			want: []Entry{
				{Line: 1, Kind: KindHeading, Level: 1, Text: "タイトル"},
				{Line: 3, Kind: KindLead, Text: "最初の文。"},
				{Line: 6, Kind: KindBullets, Text: "(箇条書き 2 項目)"},
			},
		},
		{
			name: "block kind switch without blank line flushes",
			in:   "- a\n本文です\n> 引用\n| a | b |",
			want: []Entry{
				{Line: 1, Kind: KindBullets, Text: "(箇条書き 1 項目)"},
				{Line: 2, Kind: KindLead, Text: "本文です"},
			},
		},
		{
			name: "front matter, fence and html comment are skipped",
			in:   "---\ntitle: x\n---\n```\n# not heading\n```\n<!-- # hidden -->\n## 本当の見出し ##",
			want: []Entry{
				{Line: 8, Kind: KindHeading, Level: 2, Text: "本当の見出し"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Build(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("entry %d = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestBuildHeadingStats(t *testing.T) {
	entries := []Entry{
		{Line: 1, Kind: KindHeading, Level: 1, Text: "はじめに"},
		{Line: 3, Kind: KindHeading, Level: 2, Text: "1. 設計の方針"},
		{Line: 5, Kind: KindHeading, Level: 2, Text: "実装の方針"},
		{Line: 7, Kind: KindHeading, Level: 2, Text: "APIとは"},
		{Line: 9, Kind: KindLead, Text: "本文"},
	}
	tk := fakeTokenizer{
		"はじめに":     {m("はじめ", "名詞"), m("に", "助詞")},
		"1. 設計の方針": {m("1", "名詞"), m(".", "補助記号"), m("設計", "名詞"), m("の", "助詞"), m("方針", "名詞")},
		"実装の方針":    {m("実装", "名詞"), m("の", "助詞"), m("方針", "名詞")},
		"APIとは":    {m("API", "名詞"), m("と", "助詞"), m("は", "助詞")},
	}
	stats, err := BuildHeadingStats(entries, tk)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalHeadings != 4 {
		t.Errorf("TotalHeadings = %d", stats.TotalHeadings)
	}
	if len(stats.LevelDistribution) != 2 || stats.LevelDistribution[0] != [2]int{1, 1} || stats.LevelDistribution[1] != [2]int{2, 3} {
		t.Errorf("LevelDistribution = %v", stats.LevelDistribution)
	}
	h2 := stats.ByLevel[1].Stats
	// シグネチャは 名詞×3 / 名詞×2 / 名詞×1 と全て異なる → 1/3
	if h2.DominantPOSSignatureRatio != 0.333 {
		t.Errorf("dominant ratio = %v", h2.DominantPOSSignatureRatio)
	}
	if h2.NominalEndingRatio != 0.667 {
		t.Errorf("nominal ratio = %v", h2.NominalEndingRatio)
	}
	// 連番（1.）と「とは」型
	if h2.StructuralPatternRatio != 0.667 {
		t.Errorf("structural ratio = %v", h2.StructuralPatternRatio)
	}
	if len(stats.Overall.TemplateHits) != 1 || stats.Overall.TemplateHits[0].Matched != "はじめに" {
		t.Errorf("template hits = %+v", stats.Overall.TemplateHits)
	}
	// 長さ 4, 8, 5, 5 → 平均 5.5
	if stats.Overall.LengthMean != 5.5 {
		t.Errorf("LengthMean = %v", stats.Overall.LengthMean)
	}
}

func TestMatchTemplateWordStripsNumbering(t *testing.T) {
	cases := map[string]string{
		"1. はじめに":    "はじめに",
		"【まとめ】":      "まとめ",
		"Summary of": "summary",
		"本題":         "",
	}
	for in, want := range cases {
		got, _ := matchTemplateWord(in)
		if got != want {
			t.Errorf("matchTemplateWord(%q) = %q, want %q", in, got, want)
		}
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
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		entries := Build(string(b))
		stats, err := BuildHeadingStats(entries, tk)
		if err != nil {
			t.Fatal(err)
		}
		headings := 0
		for _, e := range entries {
			if e.Kind == KindHeading {
				headings++
				if strings.HasPrefix(e.Text, "#") {
					t.Errorf("%s: heading text keeps marker: %q", f, e.Text)
				}
			}
		}
		if stats.TotalHeadings != headings {
			t.Errorf("%s: TotalHeadings = %d, want %d", f, stats.TotalHeadings, headings)
		}
	}
}
