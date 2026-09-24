package calibrate

// AllCategories は report のマトリクス行（lint が生成しうるカテゴリ）。
var AllCategories = []string{
	"forbidden_phrase",
	"translationese",
	"translationese_morph",
	"antithesis_repetition",
	"low_sentence_variance",
	"english_syntax_inanimate_subject",
	"english_syntax_cleft_because",
	"inanimate_subject_morph",
	"nominal_ending",
	"paragraph_lead_conjunction",
	"uniform_paragraph_structure",
	"low_burstiness",
	"high_length_autocorrelation",
	"repeated_sentence_lead",
	"repeated_syntax_template",
	"low_lexical_diversity_ttr",
	"low_lexical_diversity_mtld",
	"high_bold_density",
	"high_bullet_ratio",
	"boilerplate_heading",
	"numbered_phase_structure",
	"high_emoji_symbol_density",
	"low_specificity",
}

// StatisticalCategories は文書長で弁別力が変わる統計指標系（length-analysis の対象）。
var StatisticalCategories = []string{
	"low_sentence_variance",
	"low_burstiness",
	"high_length_autocorrelation",
	"nominal_ending",
	"paragraph_lead_conjunction",
	"uniform_paragraph_structure",
	"repeated_syntax_template",
	"low_lexical_diversity_ttr",
	"low_lexical_diversity_mtld",
	"low_specificity",
}

// Cell はマトリクスの 1 セル。標本 0 件の種別では率を nil にする。
type Cell struct {
	DocFireRate  *float64
	Per1000Chars *float64
	NDocs        int
	FiredDocs    int
}

// Report は検出器 × コーパス種別の発火率マトリクス。
type Report struct {
	SampleCounts map[Group]int
	// Matrix[category][group]
	Matrix map[string]map[Group]Cell
}

// BuildReport は各カテゴリについて、種別ごとの文書発火率と 1000 字あたり件数を集計する。
func BuildReport(prepared map[Group][]Prepared) Report {
	r := Report{SampleCounts: map[Group]int{}, Matrix: map[string]map[Group]Cell{}}
	for _, g := range Groups {
		r.SampleCounts[g] = len(prepared[g])
	}
	for _, cat := range AllCategories {
		row := map[Group]Cell{}
		for _, g := range Groups {
			ps := prepared[g]
			if len(ps) == 0 {
				row[g] = Cell{}
				continue
			}
			firedDocs, hits, chars := 0, 0, 0
			for _, p := range ps {
				n := countCategory(p.Findings, cat)
				if n > 0 {
					firedDocs++
				}
				hits += n
				chars += p.Doc.CharCount
			}
			rate := float64(firedDocs) / float64(len(ps))
			per := 0.0
			if chars > 0 {
				per = float64(hits) / float64(chars) * 1000
			}
			row[g] = Cell{DocFireRate: &rate, Per1000Chars: &per, NDocs: len(ps), FiredDocs: firedDocs}
		}
		r.Matrix[cat] = row
	}
	return r
}
