// Package semantic は文埋め込みによる「話題の平板さ」検出（EXPERIMENTAL・opt-in）を提供する。
//
// lint の中核パイプラインとは独立した重量級の検出器で、埋め込みモデルへの問い合わせを伴う。
// 表層の文体をどれだけ磨いても消えにくい「一つの話題を同じ意味距離で刻み続ける」癖を捉える
// （コーパス実測で perplexity・教師あり分類器・係り受けの 3 系統は不採用となり、本指標だけが残った）。
//
// 指標（埋め込みは L2 正規化済みなので内積 = cos 類似度）:
//   - coherence_flatness_range: 隣接文類似度の max-min。狭い=話題の起伏が乏しい（primary）
//   - semantic_repetition_max: 非隣接文ペア類似度の最大値（secondary/reference）
//   - topic_jump_min: 隣接文類似度の最小値（secondary/reference）
package semantic

import (
	"context"
	"fmt"
	"math"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// DefaultModel は閾値校正に使った埋め込みモデル。別モデルでは閾値の意味が変わる。
const DefaultModel = "cl-nagoya/ruri-v3-310m"

// MinSentencesForStats 未満の文数では、1〜2 箇所の類似度で range がほぼ決まってしまい
// 「起伏がない」という判定自体が統計的に不安定になるため打ち切る。
const MinSentencesForStats = 10

// 閾値は corpus/experiments/embedding/sweep-raw.json（542 文書）の再集計で、
// human_fp_base の誤検知率 <5% を保ちつつ AI 検出率が最大になる値として求めたもの。
const (
	DefaultFlatnessThreshold       = 0.16122889518737793
	defaultFlatnessCalibration     = "n(fp_base)=81, FP率=3.7%, n(ai)=405, AI検出率=85.2%（全ジャンル共通閾値）"
	SemanticRepetitionMaxThreshold = 0.9322158694267273 // value<=th
	TopicJumpMinThreshold          = 0.765757143497467  // value>=th
	categoryTopicFlatness          = "semantic_topic_flatness"
	categorySemanticRepetitionMax  = "semantic_repetition_max"
	categoryTopicJumpMin           = "topic_jump_min"
)

// GenreProfile はジャンル別に再校正した primary 指標の閾値。
type GenreProfile struct {
	FlatnessThreshold   float64
	FlatnessCalibration string
}

// GenreProfiles は essay の FP 率問題（共通閾値で 6.7%）への対応としてジャンル別に再スイープした結果。
var GenreProfiles = map[model.Genre]GenreProfile{
	model.GenreEssay: {
		FlatnessThreshold:   0.15813499689102173,
		FlatnessCalibration: "n(fp_base)=30, FP率=3.3%, n(ai)=84, AI検出率=95.2%",
	},
	model.GenreTech: {
		FlatnessThreshold:   0.14261949062347412,
		FlatnessCalibration: "n(fp_base)=18, FP率=0%, n(ai)=84, AI検出率=52.4%",
	},
	model.GenreBusiness: {
		FlatnessThreshold:   0.15565699338912964,
		FlatnessCalibration: "n(fp_base)=29, FP率=0%, n(ai)=84, AI検出率=73.8%",
	},
}

// Metrics は文書全体の類似度指標。計算できない指標は nil。
type Metrics struct {
	SemanticRepetitionMax  *float64
	CoherenceFlatnessRange *float64
	TopicJumpMin           *float64
}

// Stats は実行時の集計情報。
type Stats struct {
	Genre             model.Genre
	NSentences        int
	Model             string
	FlatnessThreshold float64
	Metrics           *Metrics
	Skipped           bool
	SkipReason        string
}

type lineSentence struct {
	line int
	text string
}

// docSentences は lint と同じ経路（マスク済みテキストで文分割→原文から同オフセットで切り出し）で
// 文を取り出す。閾値を校正した実験の文分割と揃えるため、この経路を変えてはならない。
func docSentences(rawText string) []lineSentence {
	masked := markdown.MaskStructure(rawText)
	sents := sentence.SplitWithLines(sentence.Lines(masked), sentence.LinesByNo(rawText))
	out := make([]lineSentence, 0, len(sents))
	for _, s := range sents {
		text := pystr.Strip(s.Raw)
		if text == "" {
			text = pystr.Strip(s.Masked)
		}
		if text != "" {
			out = append(out, lineSentence{line: s.Line, text: text})
		}
	}
	return out
}

// RunSemantic は rawText の話題平板性を検出する。文数が少ない場合は embedder を呼ばずにスキップする。
func RunSemantic(ctx context.Context, rawText string, genre model.Genre, embedder port.Embedder) ([]model.Finding, Stats, error) {
	threshold := DefaultFlatnessThreshold
	calibration := defaultFlatnessCalibration
	if p, ok := GenreProfiles[genre]; ok {
		threshold = p.FlatnessThreshold
		calibration = p.FlatnessCalibration
	}

	items := docSentences(rawText)
	stats := Stats{
		Genre:             genre,
		NSentences:        len(items),
		Model:             embedder.ModelName(),
		FlatnessThreshold: threshold,
	}

	if len(items) < MinSentencesForStats {
		stats.Skipped = true
		stats.SkipReason = fmt.Sprintf(
			"文数が%d文と少なく（目安%d文未満）、統計的な起伏・反復の測定が安定しないため意味的検出をスキップしました。",
			len(items), MinSentencesForStats)
		return []model.Finding{}, stats, nil
	}

	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.text
	}
	vecs, err := embedder.Embed(ctx, texts)
	if err != nil {
		return nil, stats, fmt.Errorf("embed sentences: %w", err)
	}
	if len(vecs) != len(texts) {
		return nil, stats, fmt.Errorf("embedder returned %d vectors for %d sentences", len(vecs), len(texts))
	}
	metrics := ComputeMetrics(vecs)
	stats.Metrics = &metrics

	firstLine := items[0].line
	findings := []model.Finding{}

	if cfr := metrics.CoherenceFlatnessRange; cfr != nil && *cfr <= threshold {
		findings = append(findings, model.NewFinding(
			firstLine, categoryTopicFlatness,
			fmt.Sprintf("隣接文類似度レンジ=%s（閾値%s以下）", pyfmt.Fixed(*cfr, 4), pyfmt.Fixed(threshold, 4)),
			// 実験的検出器のため critical にはしない。
			model.SeverityWarn,
			"隣接文の意味類似度の起伏が乏しい=一つの話題を同じ歩幅で刻み続ける"+
				"AI的な平板さの疑い。EXPERIMENTAL: コーパス校正では"+
				calibration+"（corpus/experiments/embedding/sweep-raw.json "+
				"542文書からの実測。genre指定なしはcorpus/reports/nn-detector-sweep.md "+
				"の全体閾値をそのまま使用）。具体例への降下・視点の転換・短い脱線で"+
				"意味的な緩急をつけることを検討してください。",
			nil,
		))
	}

	if srm := metrics.SemanticRepetitionMax; srm != nil && *srm <= SemanticRepetitionMaxThreshold {
		findings = append(findings, model.NewFinding(
			firstLine, categorySemanticRepetitionMax,
			fmt.Sprintf("非隣接文ペア類似度max=%s（参考閾値%s以下）", pyfmt.Fixed(*srm, 4), pyfmt.Fixed(SemanticRepetitionMaxThreshold, 4)),
			model.SeverityInfo,
			"参考指標（experimental・reference）。非隣接文ペアの意味的類似度の"+
				"最大値が低め＝同じ内容の言い換え反復が少ないことを示す（低いほどAI寄り、"+
				"という逆説的な弁別だが、コーパス実測ではhuman_fp_base mean=0.9878 vs "+
				"ai mean=0.9470とAI側が低い）。n(fp_base)=81, FP率=4.9%, n(ai)=405, "+
				"AI検出率=46.4%（corpus/experiments/embedding/sweep-result.md）。",
			nil,
		))
	}

	if tjm := metrics.TopicJumpMin; tjm != nil && *tjm >= TopicJumpMinThreshold {
		findings = append(findings, model.NewFinding(
			firstLine, categoryTopicJumpMin,
			fmt.Sprintf("隣接文類似度最小値=%s（参考閾値%s以上）", pyfmt.Fixed(*tjm, 4), pyfmt.Fixed(TopicJumpMinThreshold, 4)),
			model.SeverityInfo,
			"参考指標（experimental・reference）。隣接文間で意味的に大きく飛躍する"+
				"箇所が無い＝脈絡のない話題転換が少ないことを示す。"+
				"n(fp_base)=81, FP率=4.9%, n(ai)=405, AI検出率=62.2%"+
				"（corpus/experiments/embedding/sweep-result.md）。",
			nil,
		))
	}

	return findings, stats, nil
}

// ComputeMetrics は文ベクトル列から類似度指標を計算する。
// バックエンドが正規化済みベクトルを返す保証はないため、ここで改めて L2 正規化する。
func ComputeMetrics(vecs [][]float64) Metrics {
	var m Metrics
	n := len(vecs)
	if n < 3 {
		return m
	}
	unit := make([][]float64, n)
	for i, v := range vecs {
		unit[i] = normalize(v)
	}

	minAdj, maxAdj := math.Inf(1), math.Inf(-1)
	for i := 0; i+1 < n; i++ {
		s := dot(unit[i], unit[i+1])
		minAdj = math.Min(minAdj, s)
		maxAdj = math.Max(maxAdj, s)
	}
	rng := maxAdj - minAdj
	m.CoherenceFlatnessRange = &rng
	m.TopicJumpMin = &minAdj

	maxNonAdj := math.Inf(-1)
	for i := range n {
		for j := i + 2; j < n; j++ {
			maxNonAdj = math.Max(maxNonAdj, dot(unit[i], unit[j]))
		}
	}
	m.SemanticRepetitionMax = &maxNonAdj
	return m
}

func normalize(v []float64) []float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	out := make([]float64, len(v))
	norm := math.Sqrt(sum)
	if norm == 0 {
		return out
	}
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func dot(a, b []float64) float64 {
	var s float64
	for i := range min(len(a), len(b)) {
		s += a[i] * b[i]
	}
	return s
}
