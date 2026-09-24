package sentence

import (
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
)

func split(raw string) []model.Sentence {
	return SplitWithLines(Lines(markdown.MaskStructure(raw)), LinesByNo(raw))
}

func TestSplitWithLinesRawOffset(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		wantMasked string
		wantRaw    string
		wantOffset int
	}{
		{"leading code", "`code`本文です。", "本文です", "`code`本文です", 6},
		{"middle code", "前半`x`後半。", "前半   後半", "前半`x`後半", 0},
		{"trailing code", "本文`x`", "本文", "本文`x`", 0},
		{"leading full-width space", "　本文。", "本文", "本文", 0},
		{"full-width space then code", "　`x`本文。", "本文", "`x`本文", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := split(c.raw)
			if len(got) != 1 {
				t.Fatalf("got %d sentences: %+v", len(got), got)
			}
			s := got[0]
			if s.Masked != c.wantMasked || s.Raw != c.wantRaw || s.RawOffset != c.wantOffset {
				t.Errorf("got masked=%q raw=%q offset=%d", s.Masked, s.Raw, s.RawOffset)
			}
		})
	}
}

func TestSplitWithLinesSkipsStructuralLines(t *testing.T) {
	got := split("# 見出し\n- 箇条書き\n| a | b |\n本文。次の文！\n")
	if len(got) != 2 || got[0].Line != 4 || got[1].Masked != "次の文" {
		t.Fatalf("got %+v", got)
	}
}

func TestLinesSplitsOnLFOnly(t *testing.T) {
	got := Lines("a b\n\nc\n")
	if len(got) != 3 || got[0].Text != "a b" || got[2].No != 3 {
		t.Fatalf("got %+v", got)
	}
	if len(Lines("")) != 0 {
		t.Error("empty text must have no lines")
	}
}

func TestSplitWithLinesFallsBackWhenRawLengthDiffers(t *testing.T) {
	lines := []model.Line{{No: 1, Text: "本文です。"}}
	got := SplitWithLines(lines, map[int]string{1: "短い"})
	if len(got) != 1 || got[0].Raw != "本文です" {
		t.Fatalf("got %+v", got)
	}
}

func TestParagraphsAndCounts(t *testing.T) {
	paras := Paragraphs(Lines("一。二。\n続き\n\n\n三！"))
	if len(paras) != 2 || len(paras[0]) != 2 {
		t.Fatalf("paragraphs = %+v", paras)
	}
	if n := CountNonBlankPieces(JoinParagraph(paras[0])); n != 3 {
		t.Errorf("CountNonBlankPieces = %d, want 3", n)
	}
}
