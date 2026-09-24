package lint

import (
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// maxRelatedLines は 1 件の finding に載せる対応箇所の上限。同じ集計に属する finding が
// それぞれ全行番号を持つと、反復が多い文書で出力が件数の 2 乗で膨らむため。
const maxRelatedLines = 20

// formatRelatedLines は「対応箇所: L12, L34」形式に整形する（重複除去・昇順、上限を超えた分は件数だけ示す）。
func formatRelatedLines(lines []int) string {
	uniq := uniqueSorted(lines)
	shown := uniq[:min(len(uniq), maxRelatedLines)]
	parts := make([]string, len(shown))
	for i, n := range shown {
		parts[i] = fmt.Sprintf("L%d", n)
	}
	s := "対応箇所: " + strings.Join(parts, ", ")
	if rest := len(uniq) - len(shown); rest > 0 {
		s += fmt.Sprintf(" ほか%d箇所", rest)
	}
	return s
}

// capRelated は related_lines を重複除去・昇順にし、先頭 maxRelatedLines 件に切り詰める。
func capRelated(lines []int) []int {
	if lines == nil {
		return nil
	}
	uniq := uniqueSorted(lines)
	return uniq[:min(len(uniq), maxRelatedLines)]
}

func uniqueSorted(lines []int) []int {
	uniq := slices.Clone(lines)
	slices.Sort(uniq)
	return slices.Compact(uniq)
}

// excerptFrom は解析用のマスク済み行で見つけた [start, end) を原文行から切り出す（範囲外は切り詰める）。
// マスクは長さを保つので、同じオフセットがそのまま原文に使える。
func excerptFrom(raw []rune, start, end int) string {
	start = max(start, 0)
	end = min(end, len(raw))
	if start >= end {
		return ""
	}
	return string(raw[start:end])
}

// runeMatch は正規表現マッチのルーン単位の位置。
type runeMatch struct {
	start, end int
	text       string
}

// findAllRunes は re の非重複マッチをルーン単位の位置で返す（Python の re.finditer 相当）。
func findAllRunes(re *regexp.Regexp, s string) []runeMatch {
	locs := re.FindAllStringIndex(s, -1)
	out := make([]runeMatch, len(locs))
	cur := pystr.NewRuneCursor(s)
	for i, l := range locs {
		start := cur.At(l[0])
		out[i] = runeMatch{start: start, end: cur.At(l[1]), text: s[l[0]:l[1]]}
	}
	return out
}

func surfaces(ms []model.Morpheme) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Surface
	}
	return out
}

func joinSurfaces(ms []model.Morpheme) string { return strings.Join(surfaces(ms), "") }

// stripLeadingSymbols は文頭の補助記号・空白（Markdown の ** や ![ など）を除く。
func stripLeadingSymbols(ms []model.Morpheme) []model.Morpheme {
	i := 0
	for i < len(ms) && trailingSymbolPOS[ms[i].POS1()] {
		i++
	}
	return ms[i:]
}

// stripTrailingSymbols は文末の補助記号・空白（」など）を除く。
func stripTrailingSymbols(ms []model.Morpheme) []model.Morpheme {
	i := len(ms)
	for i > 0 && trailingSymbolPOS[ms[i-1].POS1()] {
		i--
	}
	return ms[:i]
}

// meanInts は整数列の平均（Python の statistics.mean と同じく正しく丸めた値）。
func meanInts(xs []int) float64 {
	var sum int64
	for _, x := range xs {
		sum += int64(x)
	}
	return float64(sum) / float64(len(xs))
}

// pstdevInts は整数列の母標準偏差。Python の statistics.pstdev は分散を有理数で厳密に求め、
// その平方根を正しく丸めて返す。float64 で割ってから Sqrt すると二重丸めで最終桁が
// ずれることがあるため、分散は整数で求め、平方根は多倍長で計算してから丸める。
func pstdevInts(xs []int) float64 {
	n := int64(len(xs))
	var s1, s2 int64
	for _, x := range xs {
		s1 += int64(x)
		s2 += int64(x) * int64(x)
	}
	num := new(big.Float).SetPrec(sqrtPrec).SetInt64(n*s2 - s1*s1)
	den := new(big.Float).SetPrec(sqrtPrec).SetInt64(n * n)
	f, _ := new(big.Float).SetPrec(sqrtPrec).Sqrt(num.Quo(num, den)).Float64()
	return f
}

const sqrtPrec = 256

// rawRunes は行 l に対応する原文行をルーン列で返す。長さがずれていればマスク済み行を使う。
func rawRunes(raw map[int]string, l model.Line) []rune {
	rs := []rune(sentence.RawOrMasked(raw, l.No, l.Text))
	if len(rs) != pystr.Len(l.Text) {
		return []rune(l.Text)
	}
	return rs
}
