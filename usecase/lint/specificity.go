package lint

import (
	"fmt"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// LowSpecificityStats は具体性判定の対象段落数と発火段落数。
type LowSpecificityStats struct {
	ParagraphsEvaluated int
	ParagraphsFired     int
}

// DetectLowSpecificity は段落単位で「具体性の欠如（一般論臭）」を検出する。
//
// 固有名詞密度・数値出現率・例示マーカーを加点し、抽象名詞率を減点した合成スコアが
// 閾値未満の段落を拾う。これは言い回しではなく素材不足のサインなので、detail では
// 書き直しより情報収集を促す。
//
// 形態素は Prepare で文ごとに解析済みのものを行番号で段落に振り分けて使い、再解析しない。
func DetectLowSpecificity(lines []model.Line, tokenized []model.TokenizedSentence, raw map[int]string, p Params) ([]model.Finding, LowSpecificityStats) {
	var findings []model.Finding
	var stats LowSpecificityStats
	byLine := map[int][]model.Morpheme{}
	for _, ts := range tokenized {
		byLine[ts.Line] = append(byLine[ts.Line], ts.Morphemes...)
	}
	for _, para := range sentence.Paragraphs(lines) {
		first := para[0]
		masked := sentence.JoinParagraph(para)
		if pystr.Len(masked) < p.LowSpecificityMinChars {
			continue
		}
		var content []model.Morpheme
		for _, l := range para {
			for _, m := range byLine[l.No] {
				if contentWordPOS[m.POS1()] {
					content = append(content, m)
				}
			}
		}
		if len(content) < p.LowSpecificityMinContentWords {
			continue
		}
		stats.ParagraphsEvaluated++

		proper, abstract := 0, 0
		for _, m := range content {
			if m.POS1() != "名詞" {
				continue
			}
			if m.POS2() == "固有名詞" {
				proper++
			}
			if AbstractNounWords[m.DictionaryForm] {
				abstract++
			}
		}
		numeric := len(numericQuantityRE.FindAllStringIndex(masked, -1))
		hasExample := false
		for _, w := range ExampleMarkerWords {
			if strings.Contains(masked, w) {
				hasExample = true
				break
			}
		}
		n := float64(len(content))
		properDensity := float64(proper) / n
		numericDensity := float64(numeric) / n
		abstractRatio := float64(abstract) / n
		bonus := 0.0
		if hasExample {
			bonus = p.LowSpecificityExampleMarkerBonus
		}
		score := properDensity*p.LowSpecificityProperNounWeight +
			numericDensity*p.LowSpecificityNumericWeight +
			bonus -
			abstractRatio*p.LowSpecificityAbstractNounWeight
		if score >= p.LowSpecificityScoreThreshold {
			continue
		}
		stats.ParagraphsFired++
		marker := "なし"
		if hasExample {
			marker = "あり"
		}
		src := sentence.RawOrMasked(raw, first.No, first.Text)
		findings = append(findings, model.NewFinding(first.No, "low_specificity", pystr.Head(pystr.Strip(src), 40),
			model.SeverityInfo,
			fmt.Sprintf("段落の具体性スコア=%s（閾値%s未満）。固有名詞密度=%s, 数値密度=%s, 抽象名詞率=%s, 例示マーカー=%s。"+
				"固有名詞・数値・実例が乏しく一般論に留まっている疑い。"+
				"素材不足のサインであり、文体の修正でなく情報収集を検討する"+
				"（revision-guide.md の素材不足の分岐を参照）",
				pyfmt.Fixed(score, 3), pyfmt.Repr(p.LowSpecificityScoreThreshold),
				pyfmt.Fixed(properDensity, 3), pyfmt.Fixed(numericDensity, 3), pyfmt.Fixed(abstractRatio, 3), marker),
			nil))
	}
	return findings, stats
}
