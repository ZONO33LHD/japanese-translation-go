// Package outline は文書のスケルトン（見出し・各段落の先頭文・箇条書き）と見出し統計を抽出する。
//
// 設計原則「検出は機械、判断はAI」に基づき、良し悪しの判断はせず決定的な抽出のみを行う。
// 見出し統計も severity 付きの指摘ではなく、読む側が判断するための材料の提示に留める。
package outline

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

// Kind はスケルトン要素の種別。
type Kind string

const (
	KindHeading Kind = "heading"
	KindLead    Kind = "lead"
	KindBullets Kind = "bullets"
)

// Entry はスケルトンの 1 要素。Level は見出しのときだけ 1〜6、それ以外は 0。
type Entry struct {
	Line  int
	Kind  Kind
	Level int
	Text  string
}

var (
	leadEndRE              = regexp.MustCompile(`[。！？]`)
	indentedContinuationRE = regexp.MustCompile(`^[` + pystr.WS + `]+[^` + pystr.WS + `]`)
)

type blockKind int

const (
	blockLead blockKind = iota
	blockBullets
	blockBlockquote
	blockTable
)

func lineKind(line string) blockKind {
	switch {
	case markdown.ListItemRE.MatchString(line):
		return blockBullets
	case markdown.BlockquoteRE.MatchString(line):
		return blockBlockquote
	case markdown.IsTableLine(line):
		return blockTable
	default:
		return blockLead
	}
}

type numberedLine struct {
	no   int
	text string
}

// Build は文書のスケルトンを行番号付きで抽出する。
//
// 見出し行そのものを残す必要があるため MaskStructure は使わず、HTML コメントだけを先に
// 空白化してから独自に走査する。ブロックの区切りは空行だけではなく、箇条書きと通常段落が
// 空行なしで切り替わった場合も確定させる（同じブロックにまとめると後続の内容が消えるため）。
func Build(rawText string) []Entry {
	lines := strings.Split(markdown.MaskHTMLComments(rawText), "\n")

	var out []Entry
	var buffer []numberedLine
	flush := func() {
		if len(buffer) == 0 {
			return
		}
		first := buffer[0]
		switch lineKind(first.text) {
		case blockBullets:
			count := 0
			for _, l := range buffer {
				if markdown.ListItemRE.MatchString(l.text) {
					count++
				}
			}
			out = append(out, Entry{Line: first.no, Kind: KindBullets, Text: fmt.Sprintf("(箇条書き %d 項目)", count)})
		case blockBlockquote, blockTable:
			// 引用と表は段落として扱わない
		default:
			lead := first.text
			if loc := leadEndRE.FindStringIndex(lead); loc != nil {
				lead = lead[:loc[1]]
			}
			if lead = pystr.Strip(lead); lead != "" {
				out = append(out, Entry{Line: first.no, Kind: KindLead, Text: lead})
			}
		}
		buffer = nil
	}

	var open *markdown.Fence
	inFrontMatter := false
	for idx, line := range lines {
		no := idx + 1
		if no == 1 && markdown.FrontMatterDelimRE.MatchString(line) {
			inFrontMatter = true
			continue
		}
		if inFrontMatter {
			if markdown.FrontMatterDelimRE.MatchString(line) {
				inFrontMatter = false
			}
			continue
		}
		if f, closeEligible, ok := markdown.FenceLine(line); ok {
			flush()
			switch {
			case open == nil:
				open = &f
			case open.Closes(f, closeEligible):
				open = nil
			}
			continue
		}
		if open != nil {
			continue
		}
		if pystr.IsBlank(line) {
			flush()
			continue
		}
		if markdown.HeadingRE.MatchString(line) {
			flush()
			level, text := markdown.HeadingLevelAndText(line)
			out = append(out, Entry{Line: no, Kind: KindHeading, Level: level, Text: text})
			continue
		}
		if len(buffer) > 0 {
			cur := lineKind(buffer[0].text)
			k := lineKind(line)
			// 折り返された箇条書き項目の継続行（字下げありでマーカーなし）は同じブロックに含める
			isContinuation := cur == blockBullets && k == blockLead && indentedContinuationRE.MatchString(line)
			if cur != k && !isContinuation {
				flush()
			}
		}
		buffer = append(buffer, numberedLine{no: no, text: line})
	}
	flush()
	return out
}
