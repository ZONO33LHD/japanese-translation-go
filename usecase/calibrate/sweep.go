package calibrate

import (
	"fmt"
	"math"
	"slices"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// maxHumanFPRate は推奨閾値の条件（人間側 FP 率がこれ未満）。
const maxHumanFPRate = 0.05

// SweepSpec は 1 検出器の閾値スイープ定義。
type SweepSpec struct {
	Category  string
	ParamName string
	Default   float64
	Values    []float64
	// IntValued は元実装で整数として扱っていた閾値（回数など）。表記を整数にそろえるために持つ。
	IntValued bool
	// Run は value を閾値にしたとき、その文書でカテゴリが 1 件以上検出されるかを返す。
	Run func(tk port.Tokenizer, p Prepared, value float64) (bool, error)
}

// frange は元実装の frange（端点を含み、各値を小数 6 桁に丸める）と同じ列を返す。
func frange(lo, hi, step float64) []float64 {
	n := int(math.RoundToEven((hi - lo) / step))
	out := make([]float64, n+1)
	for i := range out {
		out[i] = pyfmt.Round(lo+float64(i)*step, 6)
	}
	return out
}

func intRange(lo, hi int) []float64 {
	var out []float64
	for v := lo; v <= hi; v++ {
		out = append(out, float64(v))
	}
	return out
}

// withParam は閾値 1 つだけを差し替えた Params で detect を呼び、カテゴリの発火を返すスイープ関数を作る。
func withParam(category string, set func(*lint.Params, float64), detect func(port.Tokenizer, Prepared, lint.Params) ([]model.Finding, error)) func(port.Tokenizer, Prepared, float64) (bool, error) {
	return func(tk port.Tokenizer, p Prepared, v float64) (bool, error) {
		params := lint.DefaultParams()
		set(&params, v)
		fs, err := detect(tk, p, params)
		if err != nil {
			return false, err
		}
		return fired(fs, category), nil
	}
}

func rhythm(_ port.Tokenizer, p Prepared, params lint.Params) ([]model.Finding, error) {
	fs, _ := lint.DetectRhythmStatistics(p.Document.Tokenized, params)
	return fs, nil
}

func morph(_ port.Tokenizer, p Prepared, params lint.Params) ([]model.Finding, error) {
	d := p.Document
	fs, _ := lint.DetectNominalEndingAndParagraphConjunctions(d.Lines, d.Tokenized, d.RawLines, params)
	return fs, nil
}

func ngram(_ port.Tokenizer, p Prepared, params lint.Params) ([]model.Finding, error) {
	fs, _ := lint.DetectNgramRepetition(p.Document.Tokenized, params)
	return fs, nil
}

func lexdiv(_ port.Tokenizer, p Prepared, params lint.Params) ([]model.Finding, error) {
	fs, _ := lint.DetectLexicalDiversity(p.Document.Tokenized, params)
	return fs, nil
}

// SweepRegistry は sweep 可能な検出器の一覧（元実装の build_sweep_registry と同じ範囲）。
func SweepRegistry() map[string]SweepSpec {
	d := lint.DefaultParams()
	return map[string]SweepSpec{
		"low_burstiness": {
			Category: "low_burstiness", ParamName: "burstiness_threshold", Default: d.BurstinessThreshold,
			Values: frange(-0.9, -0.2, 0.02),
			Run:    withParam("low_burstiness", func(p *lint.Params, v float64) { p.BurstinessThreshold = v }, rhythm),
		},
		"high_length_autocorrelation": {
			Category: "high_length_autocorrelation", ParamName: "autocorr_threshold", Default: d.AutocorrThreshold,
			Values: frange(0.1, 0.95, 0.05),
			Run:    withParam("high_length_autocorrelation", func(p *lint.Params, v float64) { p.AutocorrThreshold = v }, rhythm),
		},
		"low_sentence_variance": {
			Category: "low_sentence_variance", ParamName: "threshold", Default: d.SentenceVarianceCVThreshold,
			Values: frange(0.05, 0.6, 0.02),
			Run: withParam("low_sentence_variance", func(p *lint.Params, v float64) { p.SentenceVarianceCVThreshold = v },
				func(_ port.Tokenizer, pr Prepared, params lint.Params) ([]model.Finding, error) {
					return lint.DetectLowSentenceLengthVariance(pr.Document.Sentences, params), nil
				}),
		},
		"nominal_ending": {
			Category: "nominal_ending", ParamName: "nominal_ratio_threshold", Default: d.NominalEndingRatioThreshold,
			Values: frange(0.05, 0.6, 0.02),
			Run:    withParam("nominal_ending", func(p *lint.Params, v float64) { p.NominalEndingRatioThreshold = v }, morph),
		},
		"paragraph_lead_conjunction": {
			Category: "paragraph_lead_conjunction", ParamName: "conj_ratio_threshold", Default: d.ParagraphConjRatioThreshold,
			Values: frange(0.05, 0.7, 0.02),
			Run:    withParam("paragraph_lead_conjunction", func(p *lint.Params, v float64) { p.ParagraphConjRatioThreshold = v }, morph),
		},
		"uniform_paragraph_structure": {
			Category: "uniform_paragraph_structure", ParamName: "uniform_cv_threshold", Default: d.UniformParagraphCVThreshold,
			Values: frange(0.02, 0.5, 0.02),
			Run:    withParam("uniform_paragraph_structure", func(p *lint.Params, v float64) { p.UniformParagraphCVThreshold = v }, morph),
		},
		"antithesis_repetition": {
			Category: "antithesis_repetition", ParamName: "threshold", Default: float64(d.AntithesisRepetitionThreshold),
			Values: intRange(1, 8), IntValued: true,
			Run: withParam("antithesis_repetition", func(p *lint.Params, v float64) { p.AntithesisRepetitionThreshold = int(v) },
				func(_ port.Tokenizer, pr Prepared, params lint.Params) ([]model.Finding, error) {
					return lint.DetectAntithesisRepetition(pr.Document.Lines, pr.Document.RawLines, params), nil
				}),
		},
		"repeated_sentence_lead": {
			Category: "repeated_sentence_lead", ParamName: "lead_repeat_threshold", Default: float64(d.NgramLeadRepeatThreshold),
			Values: intRange(1, 8), IntValued: true,
			Run: withParam("repeated_sentence_lead", func(p *lint.Params, v float64) { p.NgramLeadRepeatThreshold = int(v) }, ngram),
		},
		"repeated_syntax_template": {
			Category: "repeated_syntax_template", ParamName: "template_ratio_threshold", Default: d.NgramTemplateRatioThreshold,
			Values: frange(0.1, 0.9, 0.05),
			Run:    withParam("repeated_syntax_template", func(p *lint.Params, v float64) { p.NgramTemplateRatioThreshold = v }, ngram),
		},
		"low_lexical_diversity_ttr": {
			Category: "low_lexical_diversity_ttr", ParamName: "ttr_threshold", Default: d.TTRThreshold,
			Values: frange(0.2, 0.7, 0.02),
			Run:    withParam("low_lexical_diversity_ttr", func(p *lint.Params, v float64) { p.TTRThreshold = v }, lexdiv),
		},
		"low_lexical_diversity_mtld": {
			// 元実装では既定値・スイープ値とも整数（frange(10, 90, 2) は int のまま丸められる）。
			Category: "low_lexical_diversity_mtld", ParamName: "mtld_threshold", Default: d.MTLDThreshold,
			Values: frange(10, 90, 2), IntValued: true,
			Run: withParam("low_lexical_diversity_mtld", func(p *lint.Params, v float64) { p.MTLDThreshold = v }, lexdiv),
		},
		"low_specificity": {
			Category: "low_specificity", ParamName: "score_threshold", Default: d.LowSpecificityScoreThreshold,
			Values: frange(-0.3, 0.6, 0.02),
			Run: withParam("low_specificity", func(p *lint.Params, v float64) { p.LowSpecificityScoreThreshold = v },
				func(tk port.Tokenizer, pr Prepared, params lint.Params) ([]model.Finding, error) {
					fs, _ := lint.DetectLowSpecificity(pr.Document.Lines, pr.Document.Tokenized, pr.Document.RawLines, params)
					return fs, nil
				}),
		},
	}
}

// SweepDetectorNames は利用可能な検出器名（辞書順）。
func SweepDetectorNames() []string {
	var names []string
	for k := range SweepRegistry() {
		names = append(names, k)
	}
	slices.Sort(names)
	return names
}

// CurvePoint はスイープ 1 点分の結果。標本 0 件の側の率は nil。
type CurvePoint struct {
	Value         float64
	HumanFPRate   *float64
	HumanFPCount  int
	AIDetectRate  *float64
	AIDetectCount int
}

// SweepResult はスイープ全体の結果。
type SweepResult struct {
	Detector    string
	Spec        SweepSpec
	NHuman      int
	NAI         int
	Curve       []CurvePoint
	Recommended *CurvePoint
}

// Sweep は閾値を変えながら人間側 FP 率と AI 検出率の曲線を作る。
func Sweep(tk port.Tokenizer, name string, spec SweepSpec, human, ai []Prepared) (SweepResult, error) {
	res := SweepResult{Detector: name, Spec: spec, NHuman: len(human), NAI: len(ai)}
	for _, v := range spec.Values {
		hc, err := countFired(tk, spec, human, v)
		if err != nil {
			return SweepResult{}, err
		}
		ac, err := countFired(tk, spec, ai, v)
		if err != nil {
			return SweepResult{}, err
		}
		res.Curve = append(res.Curve, CurvePoint{
			Value: v, HumanFPRate: rate(hc, len(human)), HumanFPCount: hc,
			AIDetectRate: rate(ac, len(ai)), AIDetectCount: ac,
		})
	}
	res.Recommended = Recommend(res.Curve)
	return res, nil
}

func countFired(tk port.Tokenizer, spec SweepSpec, ps []Prepared, v float64) (int, error) {
	n := 0
	for _, p := range ps {
		ok, err := spec.Run(tk, p, v)
		if err != nil {
			return 0, fmt.Errorf("sweep %s=%v on %s: %w", spec.ParamName, v, p.Doc.Path, err)
		}
		if ok {
			n++
		}
	}
	return n, nil
}

func rate(n, total int) *float64 {
	if total == 0 {
		return nil
	}
	r := float64(n) / float64(total)
	return &r
}

// Recommend は人間側 FP 率 5% 未満の点のうち AI 検出率が最大のもの（同率なら先頭）を返す。
func Recommend(curve []CurvePoint) *CurvePoint {
	var best *CurvePoint
	bestRate := 0.0
	for i := range curve {
		c := curve[i]
		if c.HumanFPRate == nil || *c.HumanFPRate >= maxHumanFPRate {
			continue
		}
		r := 0.0
		if c.AIDetectRate != nil {
			r = *c.AIDetectRate
		}
		if best == nil || r > bestRate {
			best, bestRate = &curve[i], r
		}
	}
	if best == nil {
		return nil
	}
	cp := *best
	return &cp
}
