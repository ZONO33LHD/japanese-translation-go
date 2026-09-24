package lint

import (
	"fmt"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// DetectForbiddenPhrases は禁止語・LLM 常套句を行ごとに 1 語 1 件まで検出する。
func DetectForbiddenPhrases(lines []model.Line, raw map[int]string) []model.Finding {
	var out []model.Finding
	for _, l := range lines {
		rawLine := rawRunes(raw, l)
		for _, phrase := range ForbiddenPhrases {
			idx := pystr.Find(l.Text, phrase, 0)
			if idx < 0 {
				continue
			}
			excerpt := excerptFrom(rawLine, idx-10, idx+pystr.Len(phrase)+10)
			weak := ForbiddenPhrasesWeakSignal[phrase]
			severity := model.SeverityWarn
			detail := "禁止語/LLM常套句ヒット: 「" + phrase + "」"
			if weak {
				severity = model.SeverityInfo
				detail += "（コーパス校正で人間側にも一定数出現する弱いシグナルと判定、severity低下）"
			}
			out = append(out, model.NewFinding(l.No, "forbidden_phrase", pystr.Strip(excerpt), severity, detail, nil))
		}
	}
	return out
}

// DetectTranslationese は翻訳調の表層パターンを検出する。
func DetectTranslationese(lines []model.Line, raw map[int]string) []model.Finding {
	var out []model.Finding
	for _, l := range lines {
		rawLine := rawRunes(raw, l)
		for i, re := range translationeseREs {
			for _, m := range findAllRunes(re, l.Text) {
				excerpt := excerptFrom(rawLine, m.start-10, m.end+10)
				out = append(out, model.NewFinding(l.No, "translationese", pystr.Strip(excerpt), model.SeverityInfo,
					"翻訳調パターン: /"+TranslationesePatterns[i]+"/ に一致", nil))
			}
		}
	}
	return out
}

type antithesisHit struct {
	line    int
	excerpt string
}

// DetectAntithesisRepetition は「〜ではなく」「〜だけでなく〜も」を文書全体で数え、
// 閾値回以上なら全ヒットを反復として報告する。severity は総文数に対する比率で決める。
func DetectAntithesisRepetition(lines []model.Line, raw map[int]string, p Params) []model.Finding {
	var hits []antithesisHit
	for _, l := range lines {
		rawLine := rawRunes(raw, l)
		for _, re := range antithesisPatterns {
			for _, m := range findAllRunes(re, l.Text) {
				excerpt := excerptFrom(rawLine, m.start, m.end)
				hits = append(hits, antithesisHit{l.No, excerpt})
			}
		}
	}
	if len(hits) < p.AntithesisRepetitionThreshold {
		return nil
	}
	total := len(sentence.SplitWithLines(lines, raw))
	ratio := 0.0
	if total > 0 {
		ratio = float64(len(hits)) / float64(total)
	}
	severity := model.SeverityWarn
	switch {
	case ratio < p.AntithesisRateInfoBelow:
		severity = model.SeverityInfo
	case ratio >= p.AntithesisRateCriticalAbove:
		severity = model.SeverityCritical
	}
	all := make([]int, len(hits))
	for i, h := range hits {
		all[i] = h.line
	}
	detail := fmt.Sprintf("否定→肯定対比パターンが文書内で%d回検出（閾値%d回以上、総文数に対する比率=%s）。%s",
		len(hits), p.AntithesisRepetitionThreshold, pyfmt.Percent(ratio, 1), formatRelatedLines(all))
	out := make([]model.Finding, len(hits))
	for i, h := range hits {
		out[i] = model.NewFinding(h.line, "antithesis_repetition", pystr.Strip(h.excerpt), severity, detail, capRelated(all))
	}
	return out
}

// DetectLowSentenceLengthVariance は文長の変動係数が閾値未満（リズムが単調）なら警告する。
func DetectLowSentenceLengthVariance(sentences []model.Sentence, p Params) []model.Finding {
	var lengths []int
	for _, s := range sentences {
		if n := pystr.Len(s.Masked); n > 0 {
			lengths = append(lengths, n)
		}
	}
	if len(lengths) < p.SentenceVarianceMinSentences {
		return nil
	}
	m := meanInts(lengths)
	if m == 0 {
		return nil
	}
	cv := pstdevInts(lengths) / m
	if cv >= p.SentenceVarianceCVThreshold {
		return nil
	}
	first := 1
	if len(sentences) > 0 {
		first = sentences[0].Line
	}
	return []model.Finding{model.NewFinding(first, "low_sentence_variance",
		fmt.Sprintf("文数=%d, 平均文長=%s字, 変動係数=%s", len(lengths), pyfmt.Fixed(m, 1), pyfmt.Fixed(cv, 3)),
		model.SeverityWarn,
		fmt.Sprintf("文長の変動係数が閾値(%s)未満。リズムが均質でAI臭い可能性", pyfmt.Repr(p.SentenceVarianceCVThreshold)),
		nil)}
}

// DetectEnglishSyntaxSmell は無生物主語+他動詞と「それは〜である。なぜなら〜」構文を表層で拾う。
func DetectEnglishSyntaxSmell(lines []model.Line, raw map[int]string) []model.Finding {
	var out []model.Finding
	for _, l := range lines {
		rawLine := rawRunes(raw, l)
		for _, re := range inanimateSubjectPatterns {
			for _, m := range findAllRunes(re, l.Text) {
				excerpt := excerptFrom(rawLine, m.start, m.end)
				out = append(out, model.NewFinding(l.No, "english_syntax_inanimate_subject", excerpt, model.SeverityInfo,
					"無生物主語+他動詞的述語（表層パターン、英語統語の直訳調の可能性、要人間判断）", nil))
			}
		}
	}
	ss := sentence.SplitWithLines(lines, raw)
	for i := 0; i+1 < len(ss); i++ {
		a, b := ss[i], ss[i+1]
		if cleftBecauseHead.MatchString(a.Masked) && becauseHead.MatchString(b.Masked) {
			out = append(out, model.NewFinding(a.Line, "english_syntax_cleft_because", a.Raw+"。"+b.Raw, model.SeverityWarn,
				"「それは〜である。なぜなら〜だ」型の強調構文（英語 It is ... because ... の直訳調）", nil))
		}
	}
	return out
}
