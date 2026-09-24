package presenter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/calibrate"
)

// CalibrateReportJSON は report.json と同じ構造の値を返す。
func CalibrateReportJSON(r calibrate.Report) Object {
	counts := Object{}
	for _, g := range calibrate.Groups {
		counts = append(counts, KV{string(g), r.SampleCounts[g]})
	}
	matrix := Object{}
	for _, cat := range calibrate.AllCategories {
		row := Object{}
		for _, g := range calibrate.Groups {
			c := r.Matrix[cat][g]
			if c.NDocs == 0 {
				row = append(row, KV{string(g), Object{{"doc_fire_rate", nil}, {"per_1000_chars", nil}, {"n_docs", 0}}})
				continue
			}
			row = append(row, KV{string(g), Object{
				{"doc_fire_rate", floatPtrJSON(c.DocFireRate)},
				{"per_1000_chars", floatPtrJSON(c.Per1000Chars)},
				{"n_docs", c.NDocs},
				{"fired_docs", c.FiredDocs},
			}})
		}
		matrix = append(matrix, KV{cat, row})
	}
	return Object{{"sample_counts", counts}, {"matrix", matrix}}
}

// CalibrateReportMarkdown は report.md の本文を返す。
func CalibrateReportMarkdown(r calibrate.Report) string {
	sc := r.SampleCounts
	var b strings.Builder
	b.WriteString("# calibrate report — 検出器×コーパス種別ヒット率マトリクス\n\n")
	fmt.Fprintf(&b, "標本数: human_aozora=%d件, human_web=%d件（うち business=%d件）, ai=%d件（うち business=%d件）\n\n",
		sc[calibrate.HumanAozora], sc[calibrate.HumanWeb], sc[calibrate.HumanBusiness], sc[calibrate.AI], sc[calibrate.AIBusiness])
	b.WriteString("各セルは「文書発火率（1件以上検出した文書の割合）／1000字あたり件数」。" +
		"標本0件の種別は `-` 表記。human_business/ai_business はそれぞれ " +
		"human_web/ai の部分集合（business ジャンルのみ）で、二重集計ではなく参考列。\n\n")
	b.WriteString("| 検出器 | human_aozora | human_web | human_business | ai | ai_business |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, cat := range calibrate.AllCategories {
		row := []string{cat}
		for _, g := range calibrate.Groups {
			c := r.Matrix[cat][g]
			if c.NDocs == 0 {
				row = append(row, "-")
				continue
			}
			row = append(row, pyfmt.Percent(*c.DocFireRate, 0)+" / "+pyfmt.Fixed(*c.Per1000Chars, 2))
		}
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	b.WriteString("\n注意: 標本数が少ない種別の数値は参考値として扱うこと（標本数は冒頭の行を参照）。\n")
	return b.String()
}

// sweepValueText は閾値を元実装の str() と同じ表記にする（整数扱いの閾値は整数表記）。
func sweepValueText(v float64, intValued bool) string {
	if intValued {
		return strconv.FormatInt(int64(v), 10)
	}
	return pyfmt.Repr(v)
}

func sweepValueJSON(v float64, intValued bool) any {
	if intValued {
		return int64(v)
	}
	return pyfmt.Float(v)
}

func curvePointJSON(c calibrate.CurvePoint, intValued bool) Object {
	return Object{
		{"value", sweepValueJSON(c.Value, intValued)},
		{"human_fp_rate", floatPtrJSON(c.HumanFPRate)},
		{"human_fp_count", c.HumanFPCount},
		{"ai_detect_rate", floatPtrJSON(c.AIDetectRate)},
		{"ai_detect_count", c.AIDetectCount},
	}
}

// CalibrateSweepJSON は sweep_<detector>.json と同じ構造の値を返す。
func CalibrateSweepJSON(r calibrate.SweepResult) Object {
	iv := r.Spec.IntValued
	curve := make([]Object, len(r.Curve))
	for i, c := range r.Curve {
		curve[i] = curvePointJSON(c, iv)
	}
	var rec any
	if r.Recommended != nil {
		rec = curvePointJSON(*r.Recommended, iv)
	}
	return Object{
		{"detector", r.Detector},
		{"param_name", r.Spec.ParamName},
		{"default", sweepValueJSON(r.Spec.Default, iv)},
		{"n_human", r.NHuman},
		{"n_ai", r.NAI},
		{"curve", curve},
		{"recommended", rec},
	}
}

func optPercent(f *float64) string {
	if f == nil {
		return "-"
	}
	return pyfmt.Percent(*f, 1)
}

// CalibrateSweepMarkdown は sweep_<detector>.md の本文を返す。
func CalibrateSweepMarkdown(r calibrate.SweepResult) string {
	iv := r.Spec.IntValued
	var b strings.Builder
	fmt.Fprintf(&b, "# calibrate sweep — %s\n\n", r.Detector)
	fmt.Fprintf(&b, "パラメータ: `%s`（現行デフォルト値: %s）\n", r.Spec.ParamName, sweepValueText(r.Spec.Default, iv))
	fmt.Fprintf(&b, "標本数: human=%d件, ai=%d件\n\n", r.NHuman, r.NAI)
	b.WriteString("| 値 | 人間FP率 | AI検出率 |\n| --- | --- | --- |\n")
	for _, c := range r.Curve {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", sweepValueText(c.Value, iv), optPercent(c.HumanFPRate), optPercent(c.AIDetectRate))
	}
	b.WriteString("\n")
	if rec := r.Recommended; rec != nil {
		fmt.Fprintf(&b, "**推奨閾値**: `%s=%s`（人間FP率=%s, AI検出率=%s）\n",
			r.Spec.ParamName, sweepValueText(rec.Value, iv), optPercent(rec.HumanFPRate), optPercent(rec.AIDetectRate))
	} else {
		b.WriteString("**推奨閾値**: 人間FP率5%未満を満たす値が見つからなかった" +
			"（標本数不足、または検出器がこのコーパスでは常に人間側にも反応する可能性）\n")
	}
	b.WriteString("\n注意: human/ai の標本数が少ないうちは1件の増減でFP率/検出率が大きく動く。" +
		"コーパス規模が拡充されるまでは参考値として扱うこと。\n")
	return b.String()
}

// CalibrateLengthJSON は length_analysis.json と同じ構造の値を返す。
func CalibrateLengthJSON(r calibrate.LengthAnalysis) Object {
	bins := Object{}
	for _, lb := range calibrate.LengthBins {
		row := Object{}
		for _, cat := range calibrate.StatisticalCategories {
			c := r.Bins[lb.Label][cat]
			row = append(row, KV{cat, Object{
				{"human_fired", c.HumanFired}, {"human_total", c.HumanTotal},
				{"ai_fired", c.AIFired}, {"ai_total", c.AITotal},
			}})
		}
		bins = append(bins, KV{lb.Label, row})
	}
	minBins := Object{}
	for _, cat := range calibrate.StatisticalCategories {
		minBins = append(minBins, KV{cat, StringOrNull(r.MinEffectiveLengthBin[cat])})
	}
	return Object{{"n_human", r.NHuman}, {"n_ai", r.NAI}, {"bins", bins}, {"min_effective_length_bin", minBins}}
}

func binRate(fired, total int) string {
	if total == 0 {
		return "- (n=0)"
	}
	return fmt.Sprintf("%s (n=%d)", pyfmt.Percent(float64(fired)/float64(total), 0), total)
}

// CalibrateLengthMarkdown は length_analysis.md の本文を返す。
func CalibrateLengthMarkdown(r calibrate.LengthAnalysis) string {
	var b strings.Builder
	b.WriteString("# calibrate length-analysis — 統計指標系検出器の文書長別弁別力\n\n")
	fmt.Fprintf(&b, "標本数: human=%d件, ai=%d件\n\n", r.NHuman, r.NAI)
	for _, lb := range calibrate.LengthBins {
		fmt.Fprintf(&b, "## %s\n\n", lb.Label)
		b.WriteString("| 検出器 | human発火率(n) | ai発火率(n) |\n| --- | --- | --- |\n")
		for _, cat := range calibrate.StatisticalCategories {
			c := r.Bins[lb.Label][cat]
			fmt.Fprintf(&b, "| %s | %s | %s |\n", cat, binRate(c.HumanFired, c.HumanTotal), binRate(c.AIFired, c.AITotal))
		}
		b.WriteString("\n")
	}
	b.WriteString("## 推定「最低有効文書長」\n\n")
	b.WriteString("ai発火率がhuman発火率を上回り始める最小の文書長ビン（弁別力が正になる最小ビン）。" +
		"両側とも標本があるビンのみ判定対象。\n\n")
	b.WriteString("| 検出器 | 最低有効文書長ビン |\n| --- | --- |\n")
	for _, cat := range calibrate.StatisticalCategories {
		label := r.MinEffectiveLengthBin[cat]
		if label == "" {
			label = "判定不能（標本不足）"
		}
		fmt.Fprintf(&b, "| %s | %s |\n", cat, label)
	}
	b.WriteString("\n注意: コーパスが小規模なうちはビンごとの標本数が非常に少なく、" +
		"結果は暫定値。コーパス拡充後に再実行して確定させること。\n")
	return b.String()
}
