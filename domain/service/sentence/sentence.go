// Package sentence は行・段落・文への分割を提供する。
//
// 文分割は「。」「！」「？」「\n」を区切りとする簡易実装で、厳密な文境界解析ではない。
// マスク済みテキストで見つけた区切り位置を原文の同じオフセットに適用し、
// 解析はマスク済み、表示は原文という使い分けを可能にする。
package sentence

import (
	"regexp"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

// SplitRE は文の区切り文字。
var SplitRE = regexp.MustCompile(`[。！？\n]`)

// Lines は 1-indexed の行番号付きで行を返す。
// 区切りは LF だけにする。U+2028 や垂直タブでも区切ると、エディタや outline・構造層の
// 行番号とずれるため（入力の CRLF / CR は読み込み時に LF へ揃えている）。
func Lines(text string) []model.Line {
	raw := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if text == "" {
		raw = nil
	}
	out := make([]model.Line, len(raw))
	for i, l := range raw {
		out[i] = model.Line{No: i + 1, Text: l}
	}
	return out
}

// LinesByNo は行番号から行テキストを引くマップを作る。
func LinesByNo(text string) map[int]string {
	m := map[int]string{}
	for _, l := range Lines(text) {
		m[l.No] = l.Text
	}
	return m
}

// RawOrMasked は行番号に対応する原文行を返す（無ければ fallback）。
func RawOrMasked(raw map[int]string, no int, fallback string) string {
	if raw == nil {
		return fallback
	}
	if s, ok := raw[no]; ok {
		return s
	}
	return fallback
}

// Paragraphs は行を空行区切りの段落（行のグループ）に分ける。
func Paragraphs(lines []model.Line) [][]model.Line {
	var paras [][]model.Line
	var cur []model.Line
	for _, l := range lines {
		if !pystr.IsBlank(l.Text) {
			cur = append(cur, l)
			continue
		}
		if len(cur) > 0 {
			paras = append(paras, cur)
			cur = nil
		}
	}
	if len(cur) > 0 {
		paras = append(paras, cur)
	}
	return paras
}

// JoinParagraph は段落の行を改行で連結する。
func JoinParagraph(para []model.Line) string {
	parts := make([]string, len(para))
	for i, l := range para {
		parts[i] = l.Text
	}
	return strings.Join(parts, "\n")
}

// SplitWithLines は行ごとに文を分割し、同じオフセットで原文からも切り出す。
// 見出し・表などマスクで空になった行からは文が生成されないため、
// 原文の構造行がレポートに紛れ込むことはない。
func SplitWithLines(lines []model.Line, raw map[int]string) []model.Sentence {
	var out []model.Sentence
	for _, l := range lines {
		lineRunes := []rune(l.Text)
		rawRunes := []rune(RawOrMasked(raw, l.No, l.Text))
		if len(rawRunes) != len(lineRunes) {
			// マスクは長さを保つので通常は起きないが、ずれたオフセットで原文を切らないようにする。
			rawRunes = lineRunes
		}
		for _, b := range bounds(l.Text) {
			piece := lineRunes[b[0]:b[1]]
			masked := pystr.Strip(string(piece))
			if masked == "" {
				continue
			}
			rawPiece := rawRunes[b[0]:b[1]]
			out = append(out, model.Sentence{
				Line:      l.No,
				Masked:    masked,
				Raw:       pystr.Strip(string(rawPiece)),
				RawOffset: leadingSpaces(piece) - leadingSpaces(rawPiece),
			})
		}
	}
	return out
}

func leadingSpaces(rs []rune) int {
	n := 0
	for n < len(rs) && pystr.IsSpace(rs[n]) {
		n++
	}
	return n
}

// bounds は区切り文字で分けた各区間のルーンオフセット [start, end) を返す。
func bounds(line string) [][2]int {
	var out [][2]int
	prev := 0
	cur := pystr.NewRuneCursor(line)
	for _, m := range SplitRE.FindAllStringIndex(line, -1) {
		s := cur.At(m[0])
		out = append(out, [2]int{prev, s})
		prev = s + 1
	}
	return append(out, [2]int{prev, pystr.Len(line)})
}

// CountNonBlankPieces は SplitRE で分割した非空の断片数を返す（段落あたりの文数）。
func CountNonBlankPieces(text string) int {
	n := 0
	for _, p := range SplitRE.Split(text, -1) {
		if !pystr.IsBlank(p) {
			n++
		}
	}
	return n
}
