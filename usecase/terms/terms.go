// Package terms は専門用語候補（カタカナ複合語・ASCII 英略語・固有名詞）を初出順に抽出する。
//
// 有用な専門用語か、初出で説明済みかの判断はしない（HasGlossHint は機械的なヒントに過ぎない）。
// 文体憲法第4条（初出で説明すべき用語）の確認材料として使う。
package terms

import (
	"fmt"
	"index/suffixarray"
	"regexp"
	"slices"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

const (
	KatakanaMinLen    = 3
	GlossContextChars = 80
)

// GlossMarkerWords は初出近傍にあれば「説明の手掛かりあり」とみなす語。
var GlossMarkerWords = []string{"とは", "と呼ぶ", "という", "、つまり"}

var (
	katakanaRE = regexp.MustCompile(`^[ァ-ヶー]+$`)
	// 辞書に未登録の製品名は「普通名詞」に倒されがちなので、先頭大文字の英単語という形で補う
	capitalizedLatinRE = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)
)

// Term は用語候補 1 件。
type Term struct {
	Term         string
	FirstLine    int
	Count        int
	HasGlossHint bool
	Context      string
}

type firstSeen struct {
	line, offset int
}

// BuildInventory は専門用語候補の一覧を初出順に抽出する。
func BuildInventory(rawText string, tk port.Tokenizer) ([]Term, error) {
	maskedComments := markdown.MaskHTMLComments(rawText)
	bodyLines := sentence.Lines(markdown.MaskStructure(maskedComments))

	// 見出し行は MaskStructure で空になるため、見出しで初登場する用語を取りこぼさないよう別途合流させる
	var headingLines []model.Line
	for _, l := range sentence.Lines(maskedComments) {
		if markdown.HeadingRE.MatchString(l.Text) {
			_, text := markdown.HeadingLevelAndText(l.Text)
			headingLines = append(headingLines, model.Line{No: l.No, Text: text})
		}
	}
	combined := append(slices.Clone(bodyLines), headingLines...)
	slices.SortStableFunc(combined, func(a, b model.Line) int { return a.No - b.No })

	// maskedComments は原文と文字数・行数が一致するので、ここでのオフセットは原文にもそのまま使える
	lineStartBytes := map[int]int{}
	pos := 0
	for i, l := range strings.Split(maskedComments, "\n") {
		lineStartBytes[i+1] = pos
		pos += len(l) + 1
	}

	// 複数のパスで走査するため行内の実際の出現順は行番号だけでは決まらない。
	// 行内オフセットも記録し、最後に (行, オフセット) でソートする。
	seen := map[string]firstSeen{}
	var order []string
	register := func(term string, no, offset int) {
		term = pystr.Strip(term)
		if term == "" {
			return
		}
		if _, ok := seen[term]; !ok {
			seen[term] = firstSeen{line: no, offset: offset}
			order = append(order, term)
		}
	}

	for _, l := range combined {
		if pystr.IsBlank(l.Text) {
			continue
		}
		rs := []rune(l.Text)
		for _, span := range asciiAcronyms(rs) {
			register(string(rs[span[0]:span[1]]), l.No, span[0])
		}
		ms, err := tk.Tokenize(l.Text)
		if err != nil {
			return nil, fmt.Errorf("tokenize line %d: %w", l.No, err)
		}
		for i := 0; i < len(ms); {
			j, ok := extendRun(ms, i, isKatakanaMorpheme)
			if ok {
				term := sliceRunes(rs, ms[i].Begin, ms[j-1].End)
				if pystr.Len(term) >= KatakanaMinLen {
					register(term, l.No, ms[i].Begin)
				}
				i = j
				continue
			}
			if j, ok := extendRun(ms, i, isProperNounOrCapitalizedLatin); ok {
				register(sliceRunes(rs, ms[i].Begin, ms[j-1].End), l.No, ms[i].Begin)
				i = j
				continue
			}
			i++
		}
	}

	// 用語ごとに文書全体を分割・変換し直すと用語数×文書長になるので、1 回だけ作って共有する。
	searchLines := strings.Split(maskedComments, "\n")
	searchRunes := []rune(maskedComments)
	index := suffixarray.New([]byte(maskedComments))
	firstRunes := firstOccurrenceRunes(order, seen, index, maskedComments, searchLines, lineStartBytes)
	out := make([]Term, 0, len(order))
	for _, term := range order {
		info := seen[term]
		ctx, hint := contextAndGlossHint(term, firstRunes[term], searchLines[info.line-1], searchRunes)
		out = append(out, Term{
			Term:         term,
			FirstLine:    info.line,
			Count:        countNonOverlapping(index, term),
			HasGlossHint: hint,
			Context:      ctx,
		})
	}
	slices.SortStableFunc(out, func(a, b Term) int {
		sa, sb := seen[a.Term], seen[b.Term]
		if sa.line != sb.line {
			return sa.line - sb.line
		}
		return sa.offset - sb.offset
	})
	return out, nil
}

// extendRun は ms[i] から pred を満たす形態素が続く範囲の終端を返す。
func extendRun(ms []model.Morpheme, i int, pred func(model.Morpheme) bool) (int, bool) {
	if !pred(ms[i]) {
		return i, false
	}
	j := i + 1
	for j < len(ms) && pred(ms[j]) {
		j++
	}
	return j, true
}

func isKatakanaMorpheme(m model.Morpheme) bool { return katakanaRE.MatchString(m.Surface) }

func isProperNounOrCapitalizedLatin(m model.Morpheme) bool {
	if m.POS1() == "名詞" && m.POS2() == "固有名詞" {
		return true
	}
	return pystr.Len(m.Surface) >= 2 && capitalizedLatinRE.MatchString(m.Surface)
}

func sliceRunes(rs []rune, start, end int) string {
	start = max(0, min(start, len(rs)))
	end = max(start, min(end, len(rs)))
	return string(rs[start:end])
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// asciiAcronyms は (?<![A-Za-z0-9])[A-Z]{2,}[0-9]*(?![A-Za-z0-9]) のマッチ範囲を返す。
// 前後が ASCII 英数字でないことだけを見る（直後が日本語の「APIとは」も拾うため、\b は使わない）。
// RE2 は前後読みを持たないため走査で実装する。貪欲マッチ後に後読みが失敗した場合、
// 後退しても直後は英数字のままなので、その位置では一致しないと判定してよい。
func asciiAcronyms(rs []rune) [][2]int {
	var out [][2]int
	for i := 0; i < len(rs); {
		if i > 0 && isASCIIAlnum(rs[i-1]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && rs[j] >= 'A' && rs[j] <= 'Z' {
			j++
		}
		if j-i < 2 {
			i++
			continue
		}
		for j < len(rs) && rs[j] >= '0' && rs[j] <= '9' {
			j++
		}
		if j < len(rs) && isASCIIAlnum(rs[j]) {
			i++
			continue
		}
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

// firstOccurrenceRunes は各用語について、初出行の中で最初に現れる位置（文書全体のルーン位置）を返す。
// 見つからなければ -1。用語ごとに行を先頭から探すと、文書全体が 1 行のとき用語数×文書長になるので、
// 接尾辞配列で出現バイト位置を引き、位置の昇順に 1 回だけ走査してルーン位置へ変換する。
func firstOccurrenceRunes(order []string, seen map[string]firstSeen, index *suffixarray.Index, text string, lines []string, lineStartBytes map[int]int) map[string]int {
	type hit struct {
		term string
		b    int
	}
	hits := make([]hit, 0, len(order))
	out := make(map[string]int, len(order))
	for _, term := range order {
		line := seen[term].line
		start := lineStartBytes[line]
		end := start + len(lines[line-1])
		offsets := index.Lookup([]byte(term), -1)
		first := -1
		for _, o := range offsets {
			if o >= start && o+len(term) <= end && (first < 0 || o < first) {
				first = o
			}
		}
		out[term] = -1
		if first >= 0 {
			hits = append(hits, hit{term, first})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int { return a.b - b.b })
	cur := pystr.NewRuneCursor(text)
	for _, h := range hits {
		out[h.term] = cur.At(h.b)
	}
	return out
}

// contextAndGlossHint は用語の初出近傍（前後 GlossContextChars 字）と説明の手掛かりの有無を返す。
// 近傍にコメント内のメモ書きが紛れ込まないよう、HTML コメントを空白化したテキストを使う。
// absPos は初出のルーン位置で、-1 なら（マスク処理の副作用等で）行内に見つからなかったことを表す。
func contextAndGlossHint(term string, absPos int, lineText string, searchRunes []rune) (string, bool) {
	if absPos < 0 {
		return pystr.Strip(lineText), containsAny(lineText, GlossMarkerWords)
	}
	termLen := pystr.Len(term)
	ctxStart := max(0, absPos-GlossContextChars)
	context := sliceRunes(searchRunes, ctxStart, absPos+termLen+GlossContextChars)

	termEndLocal := absPos - ctxStart + termLen
	after := sliceRunes([]rune(context), termEndLocal, termEndLocal+2)
	hint := strings.HasPrefix(after, "(") || strings.HasPrefix(after, "（")
	if !hint {
		hint = containsAny(context, GlossMarkerWords)
	}
	return pystr.Strip(context), hint
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// countNonOverlapping は strings.Count と同じく重ならない出現回数を数える。
// 用語ごとに文書全体を走査すると用語数×文書長になるので、接尾辞配列で出現位置を引く。
func countNonOverlapping(index *suffixarray.Index, term string) int {
	if term == "" {
		return 0
	}
	offsets := index.Lookup([]byte(term), -1)
	slices.Sort(offsets)
	n, next := 0, 0
	for _, o := range offsets {
		if o >= next {
			n++
			next = o + len(term)
		}
	}
	return n
}
