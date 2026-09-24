package lint

import (
	"fmt"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// MorphStats は体言止め・段落頭接続詞・段落構造の集計。
type MorphStats struct {
	TotalSentences                int
	NominalEndingCount            int
	NominalEndingRatio            float64
	TotalParagraphs               int
	ParagraphLeadConjunctionCount int
	ParagraphLeadConjunctionRatio float64
	ParagraphSentenceCounts       []int
	// 段落数が足りず計算しなかった場合は nil。
	ParagraphSentenceCountCV *float64
}

type conjHit struct {
	line int
	text string
	conj string
}

// DetectNominalEndingAndParagraphConjunctions は
// 1) 長文なのに体言止めが 1 つもない（人間的修辞の欠如）
// 2) 段落頭の接続詞率
// 3) 段落あたり文数の均質さ（3 文段落の量産など）
// を計測する。
func DetectNominalEndingAndParagraphConjunctions(
	lines []model.Line, tokenized []model.TokenizedSentence, raw map[int]string, p Params,
) ([]model.Finding, MorphStats) {
	nominal, totalSentences, totalChars, lastLine := 0, 0, 0, 1
	for _, ts := range tokenized {
		totalSentences++
		totalChars += pystr.Len(ts.RawText)
		lastLine = ts.Line
		eff := stripTrailingSymbols(ts.Morphemes)
		if len(eff) > 0 && nounEndingPOS[eff[len(eff)-1].POS1()] {
			nominal++
		}
	}
	ratio := 0.0
	if totalSentences > 0 {
		ratio = float64(nominal) / float64(totalSentences)
	}

	var findings []model.Finding
	if totalSentences >= p.NominalEndingMinSentences && totalChars >= p.NominalEndingMinChars &&
		ratio <= p.NominalEndingRatioThreshold {
		// 欠如の検出なので指し示す文がない。一覧性のため文書末尾に 1 件だけ出す。
		findings = append(findings, model.NewFinding(lastLine, "nominal_ending",
			fmt.Sprintf("体言止め0件（全%d文、約%d字）", totalSentences, totalChars),
			model.SeverityInfo,
			"この文書には体言止めが1つもない。ある程度の長さの文書で"+
				"この修辞技法が皆無なのはAI文章に特徴的（コーパス実測: "+
				"essayジャンルで人間60% vs AI 0%が体言止めを使用）。"+
				"人間的な修辞の欠如の疑い",
			nil))
	}

	paragraphs := sentence.Paragraphs(lines)
	var conjHits []conjHit
	counts := make([]int, 0, len(paragraphs))
	for _, para := range paragraphs {
		first := pystr.Strip(para[0].Text)
		counts = append(counts, sentence.CountNonBlankPieces(sentence.JoinParagraph(para)))
		for _, conj := range ParagraphConjunctions {
			if strings.HasPrefix(first, conj) {
				conjHits = append(conjHits, conjHit{para[0].No, first, conj})
				break
			}
		}
	}
	conjRatio := 0.0
	if len(paragraphs) > 0 {
		conjRatio = float64(len(conjHits)) / float64(len(paragraphs))
	}
	if len(paragraphs) >= p.ParagraphConjMinParagraphs && conjRatio >= p.ParagraphConjRatioThreshold {
		conjLines := make([]int, len(conjHits))
		for i, h := range conjHits {
			conjLines[i] = h.line
		}
		related := formatRelatedLines(conjLines)
		for _, h := range conjHits {
			src := sentence.RawOrMasked(raw, h.line, h.text)
			findings = append(findings, model.NewFinding(h.line, "paragraph_lead_conjunction", pystr.Head(src, 40),
				model.SeverityInfo,
				fmt.Sprintf("段落頭が接続詞「%s」で始まる（文書全体の段落頭接続詞率=%s、閾値%s以上で警告）。%s",
					h.conj, pyfmt.Percent(conjRatio, 1), pyfmt.Percent(p.ParagraphConjRatioThreshold, 0), related),
				capRelated(conjLines)))
		}
	}

	stats := MorphStats{
		TotalSentences:                totalSentences,
		NominalEndingCount:            nominal,
		NominalEndingRatio:            ratio,
		TotalParagraphs:               len(paragraphs),
		ParagraphLeadConjunctionCount: len(conjHits),
		ParagraphLeadConjunctionRatio: conjRatio,
		ParagraphSentenceCounts:       counts,
	}
	if len(counts) >= p.UniformParagraphMinParagraphs {
		m := meanInts(counts)
		cv := 0.0
		if m != 0 {
			cv = pstdevInts(counts) / m
		}
		stats.ParagraphSentenceCountCV = &cv
		if cv < p.UniformParagraphCVThreshold {
			findings = append(findings, model.NewFinding(1, "uniform_paragraph_structure",
				fmt.Sprintf("段落数=%d, 各段落の文数=%s", len(counts), pyfmt.IntList(counts)),
				model.SeverityInfo,
				fmt.Sprintf("段落あたり文数の変動係数=%s（閾値%s未満）。どの段落もほぼ同じ文数=定型段落（例: 3文段落の量産）の疑い",
					pyfmt.Fixed(cv, 3), pyfmt.Repr(p.UniformParagraphCVThreshold)),
				nil))
		}
	}
	return findings, stats
}

// DetectTranslationeseMorph は「こと(名詞)+が/は(助詞)+でき〜(動詞)」の品詞列を検出する。
// 表層の正規表現と違い、送り仮名や活用（出来ます/できた 等）の揺れに影響されない。
func DetectTranslationeseMorph(tokenized []model.TokenizedSentence) []model.Finding {
	var out []model.Finding
	for _, ts := range tokenized {
		ms := ts.Morphemes
		var cut func(begin, end int) string
		for i := range ms {
			if ms[i].Surface != "こと" || ms[i].POS1() != "名詞" {
				continue
			}
			j, k := i+1, i+2
			if j >= len(ms) || ms[j].POS1() != "助詞" || (ms[j].Surface != "が" && ms[j].Surface != "は") {
				continue
			}
			if k >= len(ms) || ms[k].POS1() != "動詞" || !strings.HasPrefix(ms[k].Surface, "でき") {
				continue
			}
			// 形態素の位置はマスク済みテキスト上のオフセットなので、原文の対応位置から切り出す。
			// 切り出し関数は文ごとに 1 回だけ作る（長い 1 文でマッチごとにルーン列を作り直さない）。
			if cut == nil {
				cut = ts.Excerpter()
			}
			excerpt := clipExcerpt(cut(ms[max(0, i-4)].Begin, ms[k].End))
			out = append(out, model.NewFinding(ts.Line, "translationese_morph", excerpt, model.SeverityInfo,
				"品詞列マッチ: 名詞/動詞+こと+が/は+できる型の翻訳調構文", nil))
		}
	}
	return out
}

// DetectInanimateSubjectMorph は「抽象指示語/形式名詞 + が/は + 直訳調の他動詞」を同一文内で検出する。
// 生物性の厳密な判定は品詞だけでは困難なため、主語を抽象指示語と形式名詞に限定している。
func DetectInanimateSubjectMorph(tokenized []model.TokenizedSentence) []model.Finding {
	var out []model.Finding
	for _, ts := range tokenized {
		ms := ts.Morphemes
		n := len(ms)
		var cut func(begin, end int) string
		nextVerb := nextSmellVerbIndex(ms)
		// 「この事実」を 2 形態素でマッチさせた後に「事実」単体で再マッチさせないため。
		skipUntil := -1
		for i := range n {
			if i <= skipUntil {
				continue
			}
			isSubject := abstractPronouns[ms[i].Surface] ||
				(ms[i].POS1() == "名詞" && (ms[i].Surface == "こと" || ms[i].Surface == "事実" || ms[i].Surface == "の"))
			subjectEnd := i
			if !isSubject && i+1 < n && abstractPronounPhrases[ms[i].Surface+ms[i+1].Surface] {
				isSubject = true
				subjectEnd = i + 1
			}
			if !isSubject {
				continue
			}
			skipUntil = max(skipUntil, subjectEnd)
			j := subjectEnd + 1
			if j >= n || ms[j].POS1() != "助詞" || (ms[j].Surface != "が" && ms[j].Surface != "は") {
				continue
			}
			// 主語ごとに文末まで走査すると、動詞の無い長い文で O(n²) になるので事前計算した位置を引く。
			if k := nextVerb[j+1]; k < n {
				if cut == nil {
					cut = ts.Excerpter()
				}
				excerpt := clipExcerpt(cut(ms[max(0, i-3)].Begin, ms[k].End))
				subject := joinSurfaces(ms[i : subjectEnd+1])
				out = append(out, model.NewFinding(ts.Line, "inanimate_subject_morph", excerpt, model.SeverityInfo,
					fmt.Sprintf("品詞列マッチ: 抽象主語「%s」+ %s + 他動詞的述語「%s」（英語統語の直訳調の疑い）",
						subject, ms[j].Surface, ms[k].DictionaryForm),
					nil))
			}
		}
	}
	return out
}

// nextSmellVerbIndex は各位置 p について、p 以降で最初に現れる直訳調の他動詞の位置を返す（無ければ len(ms)）。
func nextSmellVerbIndex(ms []model.Morpheme) []int {
	next := make([]int, len(ms)+1)
	next[len(ms)] = len(ms)
	for p := len(ms) - 1; p >= 0; p-- {
		next[p] = next[p+1]
		if ms[p].POS1() == "動詞" && transitiveSmellVerbs[ms[p].DictionaryForm] {
			next[p] = p
		}
	}
	return next
}

// maxMorphExcerptRunes は品詞列マッチの excerpt の上限。主語から遠い述語までを丸ごと
// 切り出すと、長い 1 文で主語が多いときに出力が入力の 2 乗で膨らむため表示だけ切り詰める。
const maxMorphExcerptRunes = 80

func clipExcerpt(s string) string {
	rs := []rune(s)
	if len(rs) <= maxMorphExcerptRunes {
		return s
	}
	return string(rs[:maxMorphExcerptRunes]) + "…"
}
