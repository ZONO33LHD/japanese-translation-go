package presenter

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/outline"
)

// OutlineJSON は outline サブコマンドの JSON 出力を組み立てる。
func OutlineJSON(entries []outline.Entry, stats outline.HeadingStats) Object {
	es := make([]Object, len(entries))
	for i, e := range entries {
		var level any
		if e.Kind == outline.KindHeading {
			level = e.Level
		}
		es[i] = Object{{"line", e.Line}, {"kind", string(e.Kind)}, {"level", level}, {"text", e.Text}}
	}
	dist := Object{}
	for _, d := range stats.LevelDistribution {
		dist = append(dist, KV{strconv.Itoa(d[0]), d[1]})
	}
	byLevel := Object{}
	for _, l := range stats.ByLevel {
		byLevel = append(byLevel, KV{strconv.Itoa(l.Level), groupStatsJSON(l.Stats)})
	}
	return Object{
		{"outline", es},
		{"heading_stats", Object{
			{"total_headings", stats.TotalHeadings},
			{"level_distribution", dist},
			{"by_level", byLevel},
			{"overall", groupStatsJSON(stats.Overall)},
		}},
	}
}

func groupStatsJSON(g outline.GroupStats) Object {
	hits := make([]Object, len(g.TemplateHits))
	for i, h := range g.TemplateHits {
		hits[i] = Object{{"line", h.Line}, {"text", h.Text}, {"matched", h.Matched}}
	}
	return Object{
		{"count", g.Count},
		{"length_mean", pyfmt.Float(g.LengthMean)},
		{"length_cv", pyfmt.Float(g.LengthCV)},
		{"nominal_ending_ratio", pyfmt.Float(g.NominalEndingRatio)},
		{"dominant_pos_signature_ratio", pyfmt.Float(g.DominantPOSSignatureRatio)},
		{"template_hits", hits},
		{"structural_pattern_ratio", pyfmt.Float(g.StructuralPatternRatio)},
	}
}

// WriteOutlineHuman はスケルトンを人間可読形式で書き出す。
func WriteOutlineHuman(w io.Writer, path string, entries []outline.Entry) {
	fmt.Fprintf(w, "=== outline: %s ===\n\n", sanitizeTerminal(path))
	if len(entries) == 0 {
		fmt.Fprintln(w, "(スケルトンなし)")
		return
	}
	for _, e := range entries {
		tag := rightAlign("L"+strconv.Itoa(e.Line), 6)
		if e.Kind == outline.KindHeading {
			indent := strings.Repeat("  ", max(0, e.Level-1))
			fmt.Fprintf(w, "%s  %s%s %s\n", tag, indent, strings.Repeat("#", e.Level), sanitizeTerminal(e.Text))
		} else {
			fmt.Fprintf(w, "%s    %s\n", tag, sanitizeTerminal(e.Text))
		}
	}
}

func rightAlign(s string, width int) string {
	if n := pystr.Len(s); n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}

// WriteHeadingStatsHuman は見出し統計を人間可読形式で書き出す。
func WriteHeadingStatsHuman(w io.Writer, stats outline.HeadingStats) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== 見出し統計（判断材料。判定はAIが行う） ===")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "見出し総数: %d\n", stats.TotalHeadings)
	if len(stats.LevelDistribution) > 0 {
		parts := make([]string, len(stats.LevelDistribution))
		for i, d := range stats.LevelDistribution {
			parts[i] = fmt.Sprintf("h%d=%d", d[0], d[1])
		}
		fmt.Fprintf(w, "レベル分布: %s\n", strings.Join(parts, ", "))
	}
	for _, l := range stats.ByLevel {
		writeGroupHuman(w, "h"+strconv.Itoa(l.Level), l.Stats)
	}
	writeGroupHuman(w, "全体", stats.Overall)
}

func writeGroupHuman(w io.Writer, label string, g outline.GroupStats) {
	if g.Count == 0 {
		return
	}
	fmt.Fprintf(w, "[%s] 本数=%d  平均長=%s字  長さの変動係数=%s  体言止め率=%s  品詞パターン一致率=%s  構造パターン率=%s\n",
		label, g.Count, pyfmt.Repr(g.LengthMean), pyfmt.Repr(g.LengthCV),
		pyfmt.Percent(g.NominalEndingRatio, 0), pyfmt.Percent(g.DominantPOSSignatureRatio, 0),
		pyfmt.Percent(g.StructuralPatternRatio, 0))
	if len(g.TemplateHits) > 0 {
		parts := make([]string, len(g.TemplateHits))
		for i, h := range g.TemplateHits {
			parts[i] = fmt.Sprintf("L%d:%s（%s）", h.Line, sanitizeTerminal(h.Text), h.Matched)
		}
		fmt.Fprintf(w, "  テンプレ見出しヒット: %s\n", strings.Join(parts, ", "))
	}
}
