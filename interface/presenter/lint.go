package presenter

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
)

// LintReport は lint サブコマンドの出力に必要な結果一式。
type LintReport struct {
	File     string
	Findings []model.Finding
	Stats    lint.Stats
	// Baseline は --baseline 指定時だけ非 nil。
	Baseline *BaselineReport
	// ReadingLoad は --reading-load 指定時だけ非 nil。
	ReadingLoad *ReadingLoadReport
}

// BaselineReport は --baseline 比較の結果。
type BaselineReport struct {
	File     string
	Summary  lint.BaselineSummary
	Resolved []lint.BaselineFinding
}

// ReadingLoadReport は読解負荷レーンの結果。
type ReadingLoadReport struct {
	Findings []model.Finding
	Stats    lint.ReadingLoadStats
}

func genreJSON(g model.Genre) any { return StringOrNull(string(g)) }

func floatPtrJSON(f *float64) any {
	if f == nil {
		return nil
	}
	return pyfmt.Float(*f)
}

func categoryCountsJSON(cs []lint.CategoryCount) Object {
	o := Object{}
	for _, c := range cs {
		o = append(o, KV{c.Category, c.Count})
	}
	return o
}

func lintStatsJSON(s lint.Stats) Object {
	m := s.Morph
	counts := m.ParagraphSentenceCounts
	if counts == nil {
		counts = []int{}
	}
	rhythm := Object{}
	if r := s.Rhythm; r != nil {
		rhythm = Object{
			{"mora_mean", pyfmt.Float(r.MoraMean)},
			{"mora_stdev", pyfmt.Float(r.MoraStdev)},
			{"burstiness", pyfmt.Float(r.Burstiness)},
			{"length_autocorrelation_lag1", floatPtrJSON(r.LengthAutocorrelationLag1)},
		}
	}
	lex := s.LexicalDiversity
	st := s.Structural
	return Object{
		{"total_findings", s.TotalFindings},
		{"by_category", categoryCountsJSON(s.ByCategory)},
		{"genre", genreJSON(s.Genre)},
		{"experimental", s.Experimental},
		{"total_sentences", m.TotalSentences},
		{"nominal_ending_count", m.NominalEndingCount},
		{"nominal_ending_ratio", pyfmt.Float(m.NominalEndingRatio)},
		{"total_paragraphs", m.TotalParagraphs},
		{"paragraph_lead_conjunction_count", m.ParagraphLeadConjunctionCount},
		{"paragraph_lead_conjunction_ratio", pyfmt.Float(m.ParagraphLeadConjunctionRatio)},
		{"paragraph_sentence_counts", counts},
		{"paragraph_sentence_count_cv", floatPtrJSON(m.ParagraphSentenceCountCV)},
		{"rhythm", rhythm},
		{"ngram", Object{
			{"lead_pos_4gram_top", StringOrNull(s.Ngram.LeadPOS4gramTop)},
			{"lead_pos_4gram_ratio", floatPtrJSON(s.Ngram.LeadPOS4gramRatio)},
		}},
		{"lexical_diversity", Object{
			{"ttr", floatPtrJSON(lex.TTR)},
			{"mtld", floatPtrJSON(lex.MTLD)},
			{"content_token_count", lex.ContentTokenCount},
			{"doc_char_count", lex.DocCharCount},
			{"skipped_too_short", lex.SkippedTooShort},
		}},
		{"structural", Object{
			{"bold_span_count", st.BoldSpanCount},
			{"bold_per_1000_chars", pyfmt.Float(st.BoldPer1000Chars)},
			{"bullet_line_count", st.BulletLineCount},
			{"non_blank_line_count", st.NonBlankLineCount},
			{"boilerplate_heading_count", st.BoilerplateHeadingCount},
			{"numbered_phase_hit_count", st.NumberedPhaseHitCount},
			{"emoji_symbol_count", st.EmojiSymbolCount},
			{"emoji_symbol_per_1000_chars", pyfmt.Float(st.EmojiSymbolPer1000Chars)},
		}},
		{"low_specificity", Object{
			{"paragraphs_evaluated", s.LowSpecificity.ParagraphsEvaluated},
			{"paragraphs_fired", s.LowSpecificity.ParagraphsFired},
		}},
	}
}

// WriteLintJSON は元実装の --json と同じ構造・キー順で書き出す。
// baseline / reading_load セクションは指定時だけ追加し、指定しない場合の構造は変えない。
func WriteLintJSON(w io.Writer, r LintReport) error {
	out := Object{
		{"file", r.File},
		{"stats", lintStatsJSON(r.Stats)},
		{keyFindings, FindingsJSON(r.Findings)},
	}
	if b := r.Baseline; b != nil {
		resolved := make([]json.RawMessage, len(b.Resolved))
		for i, f := range b.Resolved {
			resolved[i] = baselineRawJSON(f)
		}
		out = append(out, KV{"baseline", Object{
			{"file", b.File},
			{"summary", Object{
				{"resolved", b.Summary.Resolved},
				{"new", b.Summary.New},
				{"persisting", b.Summary.Persisting},
			}},
			{"resolved", resolved},
		}})
	}
	if rl := r.ReadingLoad; rl != nil {
		out = append(out, KV{"reading_load", Object{
			{"stats", Object{
				{"total", rl.Stats.Total},
				{"sentences", rl.Stats.Sentences},
				{"genre", genreJSON(rl.Stats.Genre)},
				{"by_category", categoryCountsJSON(rl.Stats.ByCategory)},
			}},
			{"findings", FindingsJSON(rl.Findings)},
		}})
	}
	return WriteJSON(w, out)
}

// sortedByCountDesc は件数の降順（同数は初出順）に並べた複製を返す。
func sortedByCountDesc(cs []lint.CategoryCount) []lint.CategoryCount {
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, b lint.CategoryCount) int { return b.Count - a.Count })
	return out
}

// WriteLintHuman は人間可読のレポートを書き出す。
func WriteLintHuman(w io.Writer, r LintReport) {
	fmt.Fprintf(w, "=== lint: %s ===\n", sanitizeTerminal(r.File))
	fmt.Fprintf(w, "検出件数: %d\n", r.Stats.TotalFindings)
	if len(r.Stats.ByCategory) > 0 {
		fmt.Fprintln(w, "カテゴリ別内訳:")
		for _, c := range sortedByCountDesc(r.Stats.ByCategory) {
			fmt.Fprintf(w, "  - %s: %d\n", c.Category, c.Count)
		}
	}
	if b := r.Baseline; b != nil {
		fmt.Fprintf(w, "ベースライン比較: 解消: %d件 / 新規: %d件 / 継続: %d件\n", b.Summary.Resolved, b.Summary.New, b.Summary.Persisting)
	}
	fmt.Fprintln(w)
	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "検出なし。")
	} else {
		for _, f := range r.Findings {
			WriteFindingHuman(w, f)
		}
	}
	if r.ReadingLoad != nil {
		writeReadingLoadHuman(w, *r.ReadingLoad)
	}
}

func writeReadingLoadHuman(w io.Writer, r ReadingLoadReport) {
	fmt.Fprintln(w, "=== 読解負荷（推敲用の指さし・自然度スコアには含まない） ===")
	fmt.Fprintf(w, "指摘件数: %d（本文 %d 文）\n", r.Stats.Total, r.Stats.Sentences)
	if len(r.Stats.ByCategory) > 0 {
		fmt.Fprintln(w, "カテゴリ別内訳:")
		for _, c := range sortedByCountDesc(r.Stats.ByCategory) {
			fmt.Fprintf(w, "  - %s: %d\n", c.Category, c.Count)
		}
	}
	fmt.Fprintln(w)
	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "指摘なし。")
		return
	}
	for _, f := range r.Findings {
		fmt.Fprintf(w, "[指さし] L%d (%s)\n", f.Line, sanitizeTerminal(f.Category))
		fmt.Fprintf(w, "    該当箇所: %s\n", sanitizeTerminal(f.Excerpt))
		if f.Detail != "" {
			fmt.Fprintf(w, "    詳細    : %s\n", sanitizeTerminal(f.Detail))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "※ これらは「直すべき欠陥」ではなく「見るべき箇所」。読んで引っかからない文はいじらない。")
	fmt.Fprintln(w, "※ 判断は references/readability-antipatterns.md の A〜J カタログに従う。")
}
