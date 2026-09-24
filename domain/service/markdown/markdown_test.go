package markdown

import (
	"strings"
	"testing"
)

func TestMaskStructurePreservesLineCount(t *testing.T) {
	in := strings.Join([]string{
		"---",
		"title: x",
		"---",
		"# 見出し",
		"本文です。`code` を使う。",
		"- 箇条書き",
		"```go",
		"fmt.Println()",
		"```",
		"> 引用",
		"| a | b |",
		"[リンク](https://example.com)を見る。",
	}, "\n")
	got := strings.Split(MaskStructure(in), "\n")
	want := []string{
		"", "", "", "",
		"本文です。       を使う。",
		"", "", "", "", "", "",
		"[リンク](                   )を見る。",
	}
	if len(got) != len(want) {
		t.Fatalf("line count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i+1, got[i], want[i])
		}
	}
}

func TestFenceRequiresSameCharAndLength(t *testing.T) {
	in := "````\n```\nまだコード\n````\n本文。"
	got := strings.Split(MaskStructure(in), "\n")
	if got[2] != "" || got[4] != "本文。" {
		t.Errorf("unexpected mask result: %q", got)
	}
}

func TestMaskHTMLCommentsMultiline(t *testing.T) {
	in := "前<!-- a\nb -->後"
	want := "前      \n     後"
	if got := MaskHTMLComments(in); got != want {
		t.Errorf("MaskHTMLComments = %q, want %q", got, want)
	}
}

func TestBlankCodeSpansDoubleBacktick(t *testing.T) {
	cases := map[string]string{
		"a `` x`y `` b": "a           b",
		"a ``x`` b":     "a       b",
		"a `x` b `":     "a     b `",
		"``` only":      "``` only",
	}
	for in, want := range cases {
		if got := blankCodeSpans(in); got != want {
			t.Errorf("blankCodeSpans(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingLevelAndText(t *testing.T) {
	cases := []struct {
		in    string
		level int
		text  string
	}{
		{"## まとめ ##", 2, "まとめ"},
		{"# C#", 1, "C#"},
		{"###　全角空白", 3, "全角空白"},
	}
	for _, c := range cases {
		l, txt := HeadingLevelAndText(c.in)
		if l != c.level || txt != c.text {
			t.Errorf("HeadingLevelAndText(%q) = (%d, %q), want (%d, %q)", c.in, l, txt, c.level, c.text)
		}
	}
}

func TestListItemAcceptsFullWidthDigits(t *testing.T) {
	if !ListItemRE.MatchString("１. 全角数字の番号付きリスト") {
		t.Error("full-width numbered list item should match like Python's \\d")
	}
}
