package lint

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

// 読解負荷レーンは「AI臭さ」を測らない。自然度スコアにも by_category にも --baseline 比較にも
// 入れず、推敲のための指さしに留める。採否基準が弁別力ではなく「直した文が読みやすくなるか」
// なので、AI臭さの採点と同じスコアに乗せると両方の判断が濁るため。
//
// 検出器を足してよいのは「AI が読み流すと見落とすもの」だけ（網羅性のための機械検出）。
const (
	// F1: 読点区切りの名詞句がこの個数以上並び、文がこの長さ以上なら埋もれた列挙とみなす。
	readingLoadBuriedListMinItems = 3
	readingLoadBuriedListMinChars = 50
	// 3 項目のときは誤認（括弧内の列挙、並列の条件節）が混ざるので、より長い文に絞る。
	readingLoadBuriedList3ItemMinChars = 80
	readingLoadKanjiRunMax             = 6 // C1
	readingLoadNoChainMin              = 3 // C2
	// A1/A2: 「招かないとは言えません」（招か/ない/と/は/言え/ませ/ん = 距離5）まで拾える値。
	readingLoadNegationMaxGap = 6
	readingLoadNoChainMaxGap  = 3
	readingLoadExcerptLength  = 40
)

// ReadingLoadCategories は読解負荷レーンのカテゴリ。
var ReadingLoadCategories = map[string]bool{
	"sentence_too_long": true,
	"buried_list":       true,
	"kanji_run":         true,
	"double_negative":   true,
	"no_chain":          true,
}

var (
	kanjiRunRE      = regexp.MustCompile(fmt.Sprintf(`[一-鿿々]{%d,}`, readingLoadKanjiRunMax+1))
	whitespaceRunRE = regexp.MustCompile(`[` + wsClass + `]{2,}`)
)

// sudachi は形容詞「ない」を「無い」に、助動詞「ん」を「ず」に正規化するので正規化形で判定する。
var (
	negationNormalized = map[string]bool{"ない": true, "無い": true, "ぬ": true, "ず": true}
	negationPOS        = map[string]bool{"助動詞": true, "形容詞": true}
)

// 形の上では否定が二重だが、義務・必然を表す語彙化した定型で、読み手が符号の反転を計算しない表現。
// 二重否定の litotes（「ないわけではない」「ないとは言えない」）はこれらを含まないので取りこぼさない。
var obligationSpans = []string{
	"といけ", "とだめ", "とダメ", "ばならな", "ばなりま", "ばいけな",
	"てはならな", "てはなりま", "てはいけな", "ざるを得", "ざるをえ",
}

var (
	openParens  = map[string]bool{"（": true, "(": true, "「": true, "『": true, "【": true, "［": true, "[": true}
	closeParens = map[string]bool{"）": true, ")": true, "」": true, "』": true, "】": true, "］": true, "]": true}
)

// readingLength は読み手が実際に読む文字数の近似。マスクで URL・コードスパンを
// 同じ文字数の空白に置き換えているため、2 文字以上の空白を 1 文字に畳んでから数える。
func readingLength(text string) int {
	return pystr.Len(pystr.Strip(whitespaceRunRE.ReplaceAllString(text, " ")))
}

// 形態素は Begin の昇順に並ぶので、重なり始める位置を二分探索してから走査する
// （漢字の連なりごとに全形態素を舐めると、長い 1 文で O(n²) になるため）。
func spanContainsProperNoun(ms []model.Morpheme, start, end int) bool {
	i, _ := slices.BinarySearchFunc(ms, start, func(m model.Morpheme, s int) int { return cmp.Compare(m.End, s+1) })
	for ; i < len(ms) && ms[i].Begin < end; i++ {
		if ms[i].End > start && ms[i].POS2() == "固有名詞" {
			return true
		}
	}
	return false
}

func isNegation(m model.Morpheme) bool {
	return negationPOS[m.POS1()] && negationNormalized[m.NormalizedForm]
}

func isObligationForm(span string) bool {
	for _, s := range obligationSpans {
		if strings.Contains(span, s) {
			return true
		}
	}
	return false
}

// isConditionalNegation は「〜ないと動かない」「〜なければ意味がない」のような必要条件の言い方か。
// 元実装の ^(?:ない|なけれ|なく)(?:と(?!は)|ば|ければ) は否定先読みを含むので手で判定する。
// 真の litotes「〜ないとは言えない」は「と」の直後が「は」なので除外しない。
func isConditionalNegation(span string) bool {
	for _, head := range []string{"ない", "なけれ", "なく"} {
		rest, ok := strings.CutPrefix(span, head)
		if !ok {
			continue
		}
		if after, ok := strings.CutPrefix(rest, "と"); ok && !strings.HasPrefix(after, "は") {
			return true
		}
		if strings.HasPrefix(rest, "ば") || strings.HasPrefix(rest, "ければ") {
			return true
		}
	}
	return false
}

// segmentEndsWithNoun は読点で区切られた 1 区画が名詞で終わる（述語を持たない名詞句）か。
// 「確認ポイント（どのアプリが前面に出るか）」のような括弧付きの項目を拾うため、
// 末尾の括弧グループごと遡ってから品詞を見る。
func segmentEndsWithNoun(seg []model.Morpheme) bool {
	i := len(seg)
	for i > 0 {
		m := seg[i-1]
		if !trailingSymbolPOS[m.POS1()] {
			break
		}
		if !closeParens[m.Surface] {
			i--
			continue
		}
		depth := 1
		j := i - 1
		for j > 0 && depth > 0 {
			j--
			s := seg[j].Surface
			if closeParens[s] {
				depth++
			} else if openParens[s] {
				depth--
			}
		}
		i = j
	}
	return i > 0 && seg[i-1].POS1() == "名詞"
}

type nounRun struct{ start, end, items int }

// longestNounPhraseRun は名詞で終わる区画が連続する最長の並びを返す。
// 列挙の最後の項目は述語に溶ける（「A、B、C を行います」の C）ので、名詞句区画が
// MIN_ITEMS-1 個続き、その後に区画があれば、それを最後の項目とみなす。
func longestNounPhraseRun(ms []model.Morpheme) (nounRun, bool) {
	var bounds [][2]int
	start := 0
	for i, m := range ms {
		if m.Surface == "、" {
			bounds = append(bounds, [2]int{start, i})
			start = i + 1
		}
	}
	bounds = append(bounds, [2]int{start, len(ms)})

	need := readingLoadBuriedListMinItems - 1
	var best nounRun
	found := false
	var run [][2]int
	for idx, b := range bounds {
		if b[1] > b[0] && segmentEndsWithNoun(ms[b[0]:b[1]]) {
			run = append(run, b)
		} else {
			run = nil
		}
		hasTail := idx+1 < len(bounds)
		if len(run) >= need && hasTail {
			items := len(run) + 1
			if !found || items > best.items {
				best = nounRun{run[0][0], bounds[idx+1][1], items}
				found = true
			}
		}
	}
	return best, found
}

// hasPunctuationBetween は ms[i] と ms[j] のあいだに補助記号があるか。
// 読点をまたいで別の節がそれぞれ否定されている「行かないし、来ない」を二重否定と誤認しないため。
func hasPunctuationBetween(ms []model.Morpheme, i, j int) bool {
	for _, m := range ms[i+1 : j] {
		if m.POS1() == "補助記号" {
			return true
		}
	}
	return false
}

// DetectReadingLoad は読解負荷の高い箇所を指さす（判定もスコアも出さない）。
func DetectReadingLoad(tokenized []model.TokenizedSentence, sentenceMaxChars int) []model.Finding {
	var out []model.Finding
	for _, ts := range tokenized {
		text := ts.Text
		src := ts.RawText
		if src == "" {
			src = text
		}
		excerpt := pystr.Head(pystr.Strip(src), readingLoadExcerptLength)

		// B1: 一文が長すぎる
		readingLen := readingLength(text)
		if readingLen > sentenceMaxChars {
			out = append(out, model.NewFinding(ts.Line, "sentence_too_long", excerpt, model.SeverityInfo,
				fmt.Sprintf("一文が%d字（目安%d字）。カタログ B1。一文一義になっているか確認する（分割の結果、字数が増えるのは正しい）",
					readingLen, sentenceMaxChars),
				nil))
		}

		// C1: 連続漢字。固有名詞を含む連なりは分解できない名前なので除外する。
		for _, m := range findAllRunes(kanjiRunRE, text) {
			if spanContainsProperNoun(ts.Morphemes, m.start, m.end) {
				continue
			}
			out = append(out, model.NewFinding(ts.Line, "kanji_run", m.text, model.SeverityInfo,
				fmt.Sprintf("漢字が%d字連続（目安%d字）。カタログ C1。語の切れ目が読み取れるか確認する",
					pystr.Len(m.text), readingLoadKanjiRunMax),
				nil))
		}

		ms := ts.Morphemes

		// F1: 埋もれた列挙
		if run, ok := longestNounPhraseRun(ms); ok {
			minChars := readingLoadBuriedListMinChars
			if run.items <= 3 {
				minChars = readingLoadBuriedList3ItemMinChars
			}
			if readingLen >= minChars {
				out = append(out, model.NewFinding(ts.Line, "buried_list",
					pystr.Head(joinSurfaces(ms[run.start:run.end]), readingLoadExcerptLength),
					model.SeverityInfo,
					fmt.Sprintf("同格の名詞句が読点で%d個並んでいる（一文%d字）。カタログ F1。箇条書きに開くと並列関係を読み手が再構成せずに済む（「**項目**: 説明」の定型にはしない）",
						run.items, readingLen),
					nil))
			}
		}

		// A1/A2: 二重否定・否定の入れ子
		var neg []int
		for i, m := range ms {
			if isNegation(m) {
				neg = append(neg, i)
			}
		}
		for k := 0; k+1 < len(neg); k++ {
			a, b := neg[k], neg[k+1]
			span := joinSurfaces(ms[a : b+1])
			if b-a <= readingLoadNegationMaxGap && !hasPunctuationBetween(ms, a, b) &&
				!isObligationForm(span) && !isConditionalNegation(span) {
				out = append(out, model.NewFinding(ts.Line, "double_negative", span, model.SeverityInfo,
					"否定が二重に掛かっている可能性。カタログ A1/A2。"+
						"肯定に畳むなら真偽が反転していないか必ず確認する"+
						"（「招かないとは言えない」＝「招くことがある」）。"+
						"控えめな肯定が本質的な箇所は触らない",
					nil))
				break
			}
		}

		// C2: 格助詞「の」の連鎖
		var no []int
		for i, m := range ms {
			if m.Surface == "の" && m.POS1() == "助詞" && m.POS2() == "格助詞" {
				no = append(no, i)
			}
		}
		for k := 0; k+readingLoadNoChainMin <= len(no); k++ {
			window := no[k : k+readingLoadNoChainMin]
			gapsOK := true
			for w := 0; w+1 < len(window); w++ {
				if window[w+1]-window[w] > readingLoadNoChainMaxGap {
					gapsOK = false
					break
				}
			}
			if !gapsOK || hasPunctuationBetween(ms, window[0], window[len(window)-1]) {
				continue
			}
			out = append(out, model.NewFinding(ts.Line, "no_chain",
				joinSurfaces(ms[window[0]:window[len(window)-1]+1]),
				model.SeverityInfo,
				fmt.Sprintf("格助詞「の」が%d連以上。カタログ C2。どこかを動詞・連用に開く（「上限の設定の検討」→「上限をどう設定するか検討する」）",
					readingLoadNoChainMin),
				nil))
			break
		}
	}
	return model.SortFindingsByLine(out)
}
