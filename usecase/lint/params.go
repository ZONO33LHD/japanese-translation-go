package lint

import (
	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

const wsClass = pystr.WS

// Params は各検出器のヒット判定に使う閾値。calibrate が値を変えて検出器を直接呼べるよう
// 関数内のリテラルではなくここに集約している。値の根拠は元実装のコーパス校正レポートを参照。
type Params struct {
	AntithesisRepetitionThreshold int
	// 検出数/総文数の比率で severity を 3 段階化する。絶対回数だけで critical を出すと、
	// 長文書の薄い頻度や人間の修辞技法にも critical が連打されるため。
	AntithesisRateInfoBelow     float64
	AntithesisRateCriticalAbove float64

	SentenceVarianceMinSentences int
	SentenceVarianceCVThreshold  float64

	// 体言止めは人間の修辞技法だったため、「長文なのに 1 つもない」ことを検出する向きに反転している。
	NominalEndingMinSentences     int
	NominalEndingRatioThreshold   float64
	NominalEndingMinChars         int
	ParagraphConjMinParagraphs    int
	ParagraphConjRatioThreshold   float64
	UniformParagraphMinParagraphs int
	UniformParagraphCVThreshold   float64

	BurstinessMinTokenized int
	BurstinessThreshold    float64
	AutocorrMinXs          int
	AutocorrThreshold      float64

	// 文頭反復は人間の意図的な反復技法と区別できないため、明らかな過剰反復だけを拾う高めの値。
	NgramLeadRepeatThreshold    int
	NgramTemplateMinCount       int
	NgramTemplateRatioThreshold float64

	LexDivMinTokens                  int
	TTRThreshold                     float64
	MTLDThreshold                    float64
	LexDivMinDocChars                int
	LowSpecificityMinChars           int
	LowSpecificityMinContentWords    int
	LowSpecificityProperNounWeight   float64
	LowSpecificityNumericWeight      float64
	LowSpecificityExampleMarkerBonus float64
	LowSpecificityAbstractNounWeight float64
	LowSpecificityScoreThreshold     float64

	ReadingLoadSentenceMaxChars int
}

// DefaultParams はジャンル未指定時の共通の保守的閾値。
func DefaultParams() Params {
	return Params{
		AntithesisRepetitionThreshold:    3,
		AntithesisRateInfoBelow:          0.02,
		AntithesisRateCriticalAbove:      0.03,
		SentenceVarianceMinSentences:     5,
		SentenceVarianceCVThreshold:      0.25,
		NominalEndingMinSentences:        5,
		NominalEndingRatioThreshold:      0.0,
		NominalEndingMinChars:            2000,
		ParagraphConjMinParagraphs:       3,
		ParagraphConjRatioThreshold:      0.3,
		UniformParagraphMinParagraphs:    4,
		UniformParagraphCVThreshold:      0.15,
		BurstinessMinTokenized:           6,
		BurstinessThreshold:              -0.24,
		AutocorrMinXs:                    4,
		AutocorrThreshold:                0.6,
		NgramLeadRepeatThreshold:         6,
		NgramTemplateMinCount:            6,
		NgramTemplateRatioThreshold:      0.4,
		LexDivMinTokens:                  30,
		TTRThreshold:                     0.45,
		MTLDThreshold:                    40,
		LexDivMinDocChars:                4000,
		LowSpecificityMinChars:           80,
		LowSpecificityMinContentWords:    15,
		LowSpecificityProperNounWeight:   1.0,
		LowSpecificityNumericWeight:      1.0,
		LowSpecificityExampleMarkerBonus: 0.1,
		LowSpecificityAbstractNounWeight: 1.5,
		LowSpecificityScoreThreshold:     -0.15,
		ReadingLoadSentenceMaxChars:      90,
	}
}

// ForGenre はジャンル別プロファイルを適用した閾値と、そのジャンルで無効化するカテゴリを返す。
func ForGenre(g model.Genre) (Params, map[string]bool) {
	p := DefaultParams()
	switch g {
	case model.GenreEssay:
		// 体言止め欠如のシグナルが最も強く出るジャンルなので、短めの文書から拾う。
		p.NominalEndingMinChars = 1500
		p.NgramLeadRepeatThreshold = 5
		// エッセイの長い一文は書き手の呼吸であることが多い。
		p.ReadingLoadSentenceMaxChars = 110
	case model.GenreTech:
		// 人間も見出し・箇条書き構成に寄り AI との差が縮むため、誤検知を避けて緩める。
		p.NominalEndingMinChars = 3000
		p.NgramLeadRepeatThreshold = 7
		p.AntithesisRateCriticalAbove = 0.045
	case model.GenreBusiness:
		p.NominalEndingMinChars = 3000
		p.NgramLeadRepeatThreshold = 7
		// 事業文書では箇条書き・太字・定型見出し・フェーズ表現が正当に多用される。
		return p, map[string]bool{
			"high_bullet_ratio":        true,
			"high_bold_density":        true,
			"boilerplate_heading":      true,
			"numbered_phase_structure": true,
		}
	}
	return p, nil
}

// ExperimentalCategories は定量校正前、または無反応と判定された検出器。
// --experimental 指定時だけ出力する。
var ExperimentalCategories = map[string]bool{
	"high_length_autocorrelation":  true,
	"paragraph_lead_conjunction":   true,
	"repeated_syntax_template":     true,
	"english_syntax_cleft_because": true,
	"high_bold_density":            true,
	"high_bullet_ratio":            true,
	"boilerplate_heading":          true,
	"numbered_phase_structure":     true,
	"high_emoji_symbol_density":    true,
}
