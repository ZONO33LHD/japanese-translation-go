package presenter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/usecase/outline"
)

func sampleOutline() ([]outline.Entry, outline.HeadingStats) {
	entries := []outline.Entry{
		{Line: 1, Kind: outline.KindHeading, Level: 1, Text: "設計"},
		{Line: 3, Kind: outline.KindLead, Text: "本文です。"},
		{Line: 120, Kind: outline.KindBullets, Text: "(箇条書き 2 項目)"},
		{Line: 200, Kind: outline.KindHeading, Level: 2, Text: "まとめ"},
	}
	h2 := outline.GroupStats{
		Count: 1, LengthMean: 3, LengthCV: 0, NominalEndingRatio: 1, DominantPOSSignatureRatio: 1,
		TemplateHits: []outline.TemplateHit{{Line: 200, Text: "まとめ", Matched: "まとめ"}}, StructuralPatternRatio: 0,
	}
	stats := outline.HeadingStats{
		TotalHeadings:     2,
		LevelDistribution: [][2]int{{1, 1}, {2, 1}},
		ByLevel: []outline.LevelStats{
			{Level: 1, Stats: outline.GroupStats{Count: 1, LengthMean: 2, NominalEndingRatio: 1, DominantPOSSignatureRatio: 1}},
			{Level: 2, Stats: h2},
		},
		Overall: outline.GroupStats{Count: 2, LengthMean: 2.5, LengthCV: 0.2, NominalEndingRatio: 1, DominantPOSSignatureRatio: 0.5,
			TemplateHits: h2.TemplateHits},
	}
	return entries, stats
}

func TestOutlineJSON(t *testing.T) {
	entries, stats := sampleOutline()
	var buf bytes.Buffer
	if err := WriteJSON(&buf, OutlineJSON(entries, stats)); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		"{\n  \"outline\": [\n    {\n      \"line\": 1,\n      \"kind\": \"heading\",\n      \"level\": 1,\n      \"text\": \"設計\"\n    },",
		"\"kind\": \"lead\",\n      \"level\": null,",
		"\"level_distribution\": {\n      \"1\": 1,\n      \"2\": 1\n    },",
		"\"length_mean\": 3.0,",
		"\"length_cv\": 0.2,",
		"\"template_hits\": [\n          {\n            \"line\": 200,\n            \"text\": \"まとめ\",\n            \"matched\": \"まとめ\"\n          }\n        ],",
		"\"template_hits\": [],",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON missing %q:\n%s", want, got)
		}
	}
}

func TestWriteOutlineHuman(t *testing.T) {
	entries, stats := sampleOutline()
	var buf bytes.Buffer
	WriteOutlineHuman(&buf, "d.md", entries)
	WriteHeadingStatsHuman(&buf, stats)
	got := buf.String()
	for _, want := range []string{
		"=== outline: d.md ===\n\n",
		"    L1  # 設計\n",
		"    L3    本文です。\n",
		"  L120    (箇条書き 2 項目)\n",
		"  L200    ## まとめ\n",
		"見出し総数: 2\nレベル分布: h1=1, h2=1\n",
		"[h2] 本数=1  平均長=3.0字  長さの変動係数=0.0  体言止め率=100%  品詞パターン一致率=100%  構造パターン率=0%\n  テンプレ見出しヒット: L200:まとめ（まとめ）\n",
		"[全体] 本数=2  平均長=2.5字  長さの変動係数=0.2  体言止め率=100%  品詞パターン一致率=50%",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("human output missing %q:\n%s", want, got)
		}
	}
}

func TestWriteOutlineHumanEmpty(t *testing.T) {
	var buf bytes.Buffer
	WriteOutlineHuman(&buf, "e.md", nil)
	WriteHeadingStatsHuman(&buf, outline.HeadingStats{})
	got := buf.String()
	if !strings.Contains(got, "(スケルトンなし)") || !strings.Contains(got, "見出し総数: 0") {
		t.Errorf("unexpected output:\n%s", got)
	}
	if strings.Contains(got, "[全体]") || strings.Contains(got, "レベル分布") {
		t.Errorf("empty groups must not be printed:\n%s", got)
	}
}

func TestRightAlignCountsRunes(t *testing.T) {
	if got := rightAlign("L1", 6); got != "    L1" {
		t.Errorf("rightAlign = %q", got)
	}
	if got := rightAlign("L1234567", 6); got != "L1234567" {
		t.Errorf("rightAlign must not truncate: %q", got)
	}
}
