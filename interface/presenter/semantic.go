package presenter

import (
	"fmt"
	"io"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/semantic"
)

func floatPtrOrNull(p *float64) any {
	if p == nil {
		return nil
	}
	return pyfmt.Float(*p)
}

func semanticStatsJSON(st semantic.Stats) Object {
	var metrics any
	if st.Metrics != nil {
		metrics = Object{
			{"semantic_repetition_max", floatPtrOrNull(st.Metrics.SemanticRepetitionMax)},
			{"coherence_flatness_range", floatPtrOrNull(st.Metrics.CoherenceFlatnessRange)},
			{"topic_jump_min", floatPtrOrNull(st.Metrics.TopicJumpMin)},
		}
	}
	return Object{
		{"genre", StringOrNull(string(st.Genre))},
		{"n_sentences", st.NSentences},
		{"model", st.Model},
		{"flatness_threshold", pyfmt.Float(st.FlatnessThreshold)},
		{"metrics", metrics},
		{"skipped", st.Skipped},
		{"skip_reason", StringOrNull(st.SkipReason)},
	}
}

// WriteSemanticJSON は semantic の結果を JSON で書き出す。
func WriteSemanticJSON(w io.Writer, path string, findings []model.Finding, st semantic.Stats) error {
	return WriteJSON(w, Object{
		{"file", path},
		{"stats", semanticStatsJSON(st)},
		{"findings", FindingsJSON(findings)},
	})
}

// WriteSemanticHuman は semantic の結果を人間可読テキストで書き出す。
func WriteSemanticHuman(w io.Writer, path string, findings []model.Finding, st semantic.Stats) {
	fmt.Fprintf(w, "=== semantic (EXPERIMENTAL): %s ===\n", sanitizeTerminal(path))
	genre := string(st.Genre)
	if genre == "" {
		genre = "(未指定)"
	}
	fmt.Fprintf(w, "文数: %d  モデル: %s  genre: %s\n", st.NSentences, sanitizeTerminal(st.Model), genre)
	if st.Skipped {
		fmt.Fprintf(w, "スキップ: %s\n", st.SkipReason)
		return
	}
	fmt.Fprintf(w, "検出件数: %d\n", len(findings))
	fmt.Fprintln(w)
	if len(findings) == 0 {
		fmt.Fprintln(w, "検出なし。")
		return
	}
	for _, f := range findings {
		WriteFindingHuman(w, f)
	}
}
