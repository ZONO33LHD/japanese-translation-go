package lint

import (
	"fmt"
	"math"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

// RhythmStats はモーラ近似長に基づく文長リズムの統計。文数不足で計測しない場合は nil で表す。
type RhythmStats struct {
	MoraMean   float64
	MoraStdev  float64
	Burstiness float64
	// 系列が短い・分散がゼロのときは計算しないので nil。
	LengthAutocorrelationLag1 *float64
}

// 拗音を作る小書き文字は直前の文字と合わせて 1 モーラ。促音・長音は独立に数えるので含めない。
var smallKanaMerge = map[rune]bool{'ァ': true, 'ィ': true, 'ゥ': true, 'ェ': true, 'ォ': true, 'ャ': true, 'ュ': true, 'ョ': true, 'ヮ': true}

// moraLength は読み（カタカナ）からモーラ数の近似値を求める。
func moraLength(ms []model.Morpheme) int {
	total := 0
	for _, m := range ms {
		reading := m.ReadingForm
		if reading == "" {
			reading = m.Surface
		}
		count := 0
		for _, r := range reading {
			if smallKanaMerge[r] && count > 0 {
				continue
			}
			count++
		}
		total += count
	}
	return total
}

// DetectRhythmStatistics は burstiness（(σ-μ)/(σ+μ)）と隣接文長の lag-1 自己相関を計測する。
// burstiness が負に大きいほど文長が均一、自己相関が高いほど長短のパターンが固定化している。
func DetectRhythmStatistics(tokenized []model.TokenizedSentence, p Params) ([]model.Finding, *RhythmStats) {
	if len(tokenized) < p.BurstinessMinTokenized {
		return nil, nil
	}
	lengths := make([]int, len(tokenized))
	for i, ts := range tokenized {
		lengths[i] = moraLength(ts.Morphemes)
	}
	mu := meanInts(lengths)
	sigma := pstdevInts(lengths)
	burstiness := 0.0
	if sigma+mu != 0 {
		burstiness = (sigma - mu) / (sigma + mu)
	}

	xs, ys := lengths[:len(lengths)-1], lengths[1:]
	var autocorr *float64
	if len(xs) >= p.AutocorrMinXs {
		sx, sy := pstdevInts(xs), pstdevInts(ys)
		if sx > 0 && sy > 0 {
			mx, my := meanInts(xs), meanInts(ys)
			var cov float64
			for i := range xs {
				cov += (float64(xs[i]) - mx) * (float64(ys[i]) - my)
			}
			v := cov / float64(len(xs)) / (sx * sy)
			autocorr = &v
		}
	}

	var findings []model.Finding
	first := tokenized[0].Line
	if burstiness < p.BurstinessThreshold {
		findings = append(findings, model.NewFinding(first, "low_burstiness",
			fmt.Sprintf("burstiness=%s (モーラ近似長 平均=%s, 標準偏差=%s)", pyfmt.Fixed(burstiness, 3), pyfmt.Fixed(mu, 1), pyfmt.Fixed(sigma, 1)),
			model.SeverityWarn,
			fmt.Sprintf("burstiness が閾値(%s)未満。文の長短のメリハリが乏しく機械的なリズムの疑い", pyfmt.Repr(p.BurstinessThreshold)),
			nil))
	}
	if autocorr != nil && *autocorr > p.AutocorrThreshold {
		findings = append(findings, model.NewFinding(first, "high_length_autocorrelation",
			"lag-1 自己相関="+pyfmt.Fixed(*autocorr, 3),
			model.SeverityInfo,
			fmt.Sprintf("隣接する文の長さが強く相関（閾値%s超）。文長パターンが単調に繰り返されている疑い", pyfmt.Repr(p.AutocorrThreshold)),
			nil))
	}
	return findings, &RhythmStats{MoraMean: mu, MoraStdev: sigma, Burstiness: burstiness, LengthAutocorrelationLag1: autocorr}
}

// NgramStats は文頭品詞 4-gram の最頻パターンとその一致率。対象文が少なければ未計算（空・nil）。
type NgramStats struct {
	LeadPOS4gramTop   string
	LeadPOS4gramRatio *float64
}

type leadBigram struct {
	line   int
	raw    string
	text   string
	isTech bool
}

// isProperNounOrTechTerm は文頭形態素が固有名詞、またはラテン文字主体の製品名/ライブラリ名かを判定する。
// カタカナ語は一般語も多く、誤って説明を変えるリスクが高いので対象外。
func isProperNounOrTechTerm(m model.Morpheme) bool {
	return (m.POS1() == "名詞" && m.POS2() == "固有名詞") || latinTechTokenRE.MatchString(m.Surface)
}

// DetectNgramRepetition は
// 1) 文頭 2 形態素の反復（「そして、」「また、」の使い回し）
// 2) 文頭品詞 4-gram の一致率（語彙は違っても構文テンプレートが同じ）
// を検出する。
func DetectNgramRepetition(tokenized []model.TokenizedSentence, p Params) ([]model.Finding, NgramStats) {
	leads := detectRepeatedSentenceLeads(tokenized, p)
	templates, stats := detectRepeatedSyntaxTemplate(tokenized, p)
	return append(leads, templates...), stats
}

func detectRepeatedSentenceLeads(tokenized []model.TokenizedSentence, p Params) []model.Finding {
	var findings []model.Finding

	// 文頭の補助記号を落としてから数える。インラインの ** や ![ が文頭に残り、
	// 無意味な反復が量産されるのを防ぐため。
	var bigrams []leadBigram
	for _, ts := range tokenized {
		lead := stripLeadingSymbols(ts.Morphemes)
		if len(lead) < 2 {
			continue
		}
		lead = lead[:2]
		bigrams = append(bigrams, leadBigram{ts.Line, ts.RawText, joinSurfaces(lead), isProperNounOrTechTerm(lead[0])})
	}
	counts, order := countInOrder(bigrams, func(b leadBigram) string { return b.text })
	for _, bigram := range order {
		count := counts[bigram]
		if count < p.NgramLeadRepeatThreshold {
			continue
		}
		var lines []int
		for _, b := range bigrams {
			if b.text == bigram {
				lines = append(lines, b.line)
			}
		}
		related := formatRelatedLines(lines)
		for _, b := range bigrams {
			if b.text != bigram {
				continue
			}
			reason := "人間の意図的な反復技法との区別がつかないため参考情報として提示。"
			if b.isTech {
				reason = "固有名詞/技術用語由来の可能性が高い。"
			}
			// 人間の意図的な反復と区別できないため severity は常に info。
			findings = append(findings, model.NewFinding(b.line, "repeated_sentence_lead", pystr.Head(b.raw, 20),
				model.SeverityInfo,
				fmt.Sprintf("文頭2形態素「%s」が%d回反復（閾値%d回以上）。%s%s", bigram, count, p.NgramLeadRepeatThreshold, reason, related),
				capRelated(lines)))
		}
	}
	return findings
}

func detectRepeatedSyntaxTemplate(tokenized []model.TokenizedSentence, p Params) ([]model.Finding, NgramStats) {
	var findings []model.Finding
	type posGram struct {
		line int
		raw  string
		seq  string
	}
	var grams []posGram
	for _, ts := range tokenized {
		lead := stripLeadingSymbols(ts.Morphemes)
		if len(lead) < 4 {
			continue
		}
		pos := make([]string, 4)
		for i, m := range lead[:4] {
			pos[i] = m.POS1()
		}
		grams = append(grams, posGram{ts.Line, ts.RawText, strings.Join(pos, "/")})
	}

	var stats NgramStats
	posCounts, posOrder := countInOrder(grams, func(g posGram) string { return g.seq })
	if len(grams) < p.NgramTemplateMinCount || len(posOrder) == 0 {
		return findings, stats
	}
	// Counter.most_common(1) は同数なら最初に出現したものを返す。
	top := posOrder[0]
	for _, s := range posOrder[1:] {
		if posCounts[s] > posCounts[top] {
			top = s
		}
	}
	ratio := float64(posCounts[top]) / float64(len(grams))
	stats.LeadPOS4gramTop = top
	stats.LeadPOS4gramRatio = &ratio
	if ratio < p.NgramTemplateRatioThreshold {
		return findings, stats
	}
	var lines []int
	for _, g := range grams {
		if g.seq == top {
			lines = append(lines, g.line)
		}
	}
	related := formatRelatedLines(lines)
	for _, g := range grams {
		if g.seq != top {
			continue
		}
		findings = append(findings, model.NewFinding(g.line, "repeated_syntax_template", pystr.Head(g.raw, 20),
			model.SeverityInfo,
			fmt.Sprintf("文頭品詞4-gram「%s」が全文の%sで一致（閾値%s以上）。構文テンプレートの使い回しの疑い。%s",
				top, pyfmt.Percent(ratio, 1), pyfmt.Percent(p.NgramTemplateRatioThreshold, 0), related),
			capRelated(lines)))
	}
	return findings, stats
}

// countInOrder は出現回数と初出順のキー列を返す（Python の Counter の挿入順を再現する）。
func countInOrder[T any](xs []T, key func(T) string) (map[string]int, []string) {
	counts := map[string]int{}
	var order []string
	for _, x := range xs {
		k := key(x)
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
	}
	return counts, order
}

// computeMTLD は MTLD（Measure of Textual Lexical Diversity）の簡易実装。
// TTR が threshold を下回るたびに 1 ファクターと数え、前方・後方の平均をとる。
func computeMTLD(tokens []string, threshold float64) *float64 {
	if len(tokens) < 20 {
		return nil
	}
	v := (mtldOneDirection(tokens, threshold) + mtldOneDirection(pystr.Reverse(tokens), threshold)) / 2
	return &v
}

func mtldOneDirection(seq []string, threshold float64) float64 {
	factors := 0.0
	types := map[string]bool{}
	count := 0
	for _, tok := range seq {
		types[tok] = true
		count++
		if float64(len(types))/float64(count) <= threshold {
			factors++
			types = map[string]bool{}
			count = 0
		}
	}
	if count > 0 {
		ttr := float64(len(types)) / float64(count)
		partial := 0.0
		if ttr < 1 {
			partial = (1 - ttr) / (1 - threshold)
		}
		factors += math.Min(partial, 1.0)
	}
	if factors > 0 {
		return float64(len(seq)) / factors
	}
	return float64(len(seq))
}

// LexicalDiversityStats は内容語の TTR / MTLD。文書が短い場合は SkippedTooShort。
type LexicalDiversityStats struct {
	TTR               *float64
	MTLD              *float64
	ContentTokenCount int
	DocCharCount      int
	SkippedTooShort   bool
}

// DetectLexicalDiversity は内容語の辞書形を対象に TTR と MTLD を計測する。
// TTR は短い文書では人間/AI とも差が出ないため、文書長の適用条件でガードする。
func DetectLexicalDiversity(tokenized []model.TokenizedSentence, p Params) ([]model.Finding, LexicalDiversityStats) {
	var tokens []string
	docChars := 0
	for _, ts := range tokenized {
		docChars += pystr.Len(ts.RawText)
		for _, m := range ts.Morphemes {
			if contentWordPOS[m.POS1()] {
				tokens = append(tokens, m.DictionaryForm)
			}
		}
	}
	stats := LexicalDiversityStats{ContentTokenCount: len(tokens), DocCharCount: docChars}
	if docChars < p.LexDivMinDocChars {
		stats.SkippedTooShort = true
		return nil, stats
	}
	if len(tokens) < p.LexDivMinTokens {
		return nil, stats
	}
	types := map[string]bool{}
	for _, t := range tokens {
		types[t] = true
	}
	ttr := float64(len(types)) / float64(len(tokens))
	mtld := computeMTLD(tokens, 0.72)
	stats.TTR = &ttr
	stats.MTLD = mtld

	var findings []model.Finding
	first := tokenized[0].Line
	if ttr < p.TTRThreshold {
		findings = append(findings, model.NewFinding(first, "low_lexical_diversity_ttr",
			fmt.Sprintf("TTR=%s (内容語 %d 語中 %d 種類)", pyfmt.Fixed(ttr, 3), len(tokens), len(types)),
			model.SeverityInfo,
			fmt.Sprintf("TTR(Type-Token Ratio)が閾値%s未満。同じ語彙の使い回しが多い疑い", pyfmt.Repr(p.TTRThreshold)),
			nil))
	}
	if mtld != nil && *mtld < p.MTLDThreshold {
		findings = append(findings, model.NewFinding(first, "low_lexical_diversity_mtld",
			"MTLD="+pyfmt.Fixed(*mtld, 1),
			model.SeverityInfo,
			fmt.Sprintf("MTLD が閾値%s未満。文章長で正規化した語彙多様性が低い疑い", formatThreshold(p.MTLDThreshold)),
			nil))
	}
	return findings, stats
}

// formatThreshold は元実装で int リテラルだった閾値（MTLD の 40 など）を整数表記で出す。
func formatThreshold(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return fmt.Sprintf("%d", int64(f))
	}
	return pyfmt.Repr(f)
}
