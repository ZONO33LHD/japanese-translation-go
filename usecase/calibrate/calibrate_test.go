package calibrate

import (
	"slices"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

func TestFrangeMatchesPython(t *testing.T) {
	got := frange(-0.9, -0.2, 0.02)
	if len(got) != 36 || got[0] != -0.9 || got[len(got)-1] != -0.2 || got[1] != -0.88 {
		t.Errorf("frange(-0.9,-0.2,0.02) = %v", got)
	}
	if got := frange(10, 90, 2); len(got) != 41 || got[40] != 90 {
		t.Errorf("frange(10,90,2) len=%d last=%v", len(got), got[len(got)-1])
	}
	if got := frange(0.1, 0.95, 0.05); len(got) != 18 || got[17] != 0.95 {
		t.Errorf("frange(0.1,0.95,0.05) = %v", got)
	}
}

func ptr(f float64) *float64 { return &f }

func TestRecommend(t *testing.T) {
	cases := []struct {
		name  string
		curve []CurvePoint
		want  *float64
	}{
		{"picks max AI rate under 5% FP", []CurvePoint{
			{Value: 1, HumanFPRate: ptr(0.01), AIDetectRate: ptr(0.3)},
			{Value: 2, HumanFPRate: ptr(0.04), AIDetectRate: ptr(0.6)},
			{Value: 3, HumanFPRate: ptr(0.05), AIDetectRate: ptr(0.9)},
		}, ptr(2)},
		{"first wins on tie", []CurvePoint{
			{Value: 1, HumanFPRate: ptr(0), AIDetectRate: ptr(0.5)},
			{Value: 2, HumanFPRate: ptr(0), AIDetectRate: ptr(0.5)},
		}, ptr(1)},
		{"nil AI rate counts as zero", []CurvePoint{
			{Value: 1, HumanFPRate: ptr(0), AIDetectRate: nil},
		}, ptr(1)},
		{"no human samples", []CurvePoint{{Value: 1}}, nil},
		{"all above FP limit", []CurvePoint{{Value: 1, HumanFPRate: ptr(0.2), AIDetectRate: ptr(1)}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Recommend(c.curve)
			switch {
			case c.want == nil && got != nil:
				t.Errorf("got %v, want nil", got.Value)
			case c.want != nil && (got == nil || got.Value != *c.want):
				t.Errorf("got %v, want %v", got, *c.want)
			}
		})
	}
}

func TestSweepCountsRates(t *testing.T) {
	spec := SweepSpec{
		ParamName: "x",
		Values:    []float64{1, 2, 3},
		// 文字数が閾値以上なら発火する合成の検出器。
		Run: func(_ port.Tokenizer, p Prepared, v float64) (bool, error) { return float64(p.Doc.CharCount) >= v, nil },
	}
	human := []Prepared{{Doc: Doc{CharCount: 1}}, {Doc: Doc{CharCount: 3}}}
	ai := []Prepared{{Doc: Doc{CharCount: 2}}, {Doc: Doc{CharCount: 3}}}
	r, err := Sweep(nil, "x", spec, human, ai)
	if err != nil {
		t.Fatal(err)
	}
	gotHuman := []int{r.Curve[0].HumanFPCount, r.Curve[1].HumanFPCount, r.Curve[2].HumanFPCount}
	gotAI := []int{r.Curve[0].AIDetectCount, r.Curve[1].AIDetectCount, r.Curve[2].AIDetectCount}
	if !slices.Equal(gotHuman, []int{2, 1, 1}) || !slices.Equal(gotAI, []int{2, 2, 1}) {
		t.Errorf("human=%v ai=%v", gotHuman, gotAI)
	}
	if r.Recommended != nil {
		t.Errorf("recommended = %v, want nil (human FP is 50%% everywhere)", r.Recommended.Value)
	}

	empty, err := Sweep(nil, "x", spec, nil, ai)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Curve[0].HumanFPRate != nil {
		t.Error("human rate should be nil without human samples")
	}
}

func TestSweepRegistryCoversPythonDetectors(t *testing.T) {
	want := []string{
		"antithesis_repetition", "high_length_autocorrelation", "low_burstiness", "low_lexical_diversity_mtld",
		"low_lexical_diversity_ttr", "low_sentence_variance", "low_specificity", "nominal_ending",
		"paragraph_lead_conjunction", "repeated_sentence_lead", "repeated_syntax_template", "uniform_paragraph_structure",
	}
	if got := SweepDetectorNames(); !slices.Equal(got, want) {
		t.Errorf("detectors = %v", got)
	}
}

func TestBinForLength(t *testing.T) {
	cases := map[int]string{0: "~1000字", 999: "~1000字", 1000: "~2000字", 3999: "~4000字", 4000: "4000字~", 100000: "4000字~"}
	for n, want := range cases {
		if got := BinForLength(n); got != want {
			t.Errorf("BinForLength(%d) = %q, want %q", n, got, want)
		}
	}
}

func finding(cat string) model.Finding { return model.Finding{Category: cat} }

func TestAnalyzeLengthMinEffectiveBin(t *testing.T) {
	human := []Prepared{
		{Doc: Doc{CharCount: 500}, Findings: []model.Finding{finding("low_burstiness")}},
		{Doc: Doc{CharCount: 5000}},
	}
	ai := []Prepared{
		{Doc: Doc{CharCount: 500}, Findings: []model.Finding{finding("low_burstiness")}},
		{Doc: Doc{CharCount: 5000}, Findings: []model.Finding{finding("low_burstiness")}},
	}
	r := AnalyzeLength(human, ai)
	if c := r.Bins["~1000字"]["low_burstiness"]; c != (BinCell{HumanFired: 1, HumanTotal: 1, AIFired: 1, AITotal: 1}) {
		t.Errorf("~1000字 cell = %+v", c)
	}
	// ~1000字 は同率なので弁別力なし、4000字~ で初めて AI が上回る。
	if got := r.MinEffectiveLengthBin["low_burstiness"]; got != "4000字~" {
		t.Errorf("min bin = %q", got)
	}
	if got := r.MinEffectiveLengthBin["low_specificity"]; got != "" {
		t.Errorf("min bin for never-firing detector = %q, want empty", got)
	}
}

func TestBuildReport(t *testing.T) {
	prepared := map[Group][]Prepared{
		HumanAozora: {
			{Doc: Doc{CharCount: 1000}, Findings: []model.Finding{finding("forbidden_phrase"), finding("forbidden_phrase")}},
			{Doc: Doc{CharCount: 1000}},
		},
	}
	r := BuildReport(prepared)
	c := r.Matrix["forbidden_phrase"][HumanAozora]
	if c.NDocs != 2 || c.FiredDocs != 1 || *c.DocFireRate != 0.5 || *c.Per1000Chars != 1.0 {
		t.Errorf("cell = %+v rate=%v per=%v", c, *c.DocFireRate, *c.Per1000Chars)
	}
	if c := r.Matrix["forbidden_phrase"][AI]; c.NDocs != 0 || c.DocFireRate != nil {
		t.Errorf("empty group cell = %+v", c)
	}
	if r.SampleCounts[HumanAozora] != 2 || r.SampleCounts[AI] != 0 {
		t.Errorf("sample counts = %v", r.SampleCounts)
	}
}

func TestGroupCorpusBusinessSubsets(t *testing.T) {
	files := port.CorpusFiles{
		Aozora: []port.CorpusDocument{{Path: "human/aozora/a.txt", Text: "あいう"}},
		Web: []port.CorpusDocument{
			{Path: "human/web/biz-1.md", Text: "x"},
			{Path: "human/web/note-1.md", Text: "y"},
		},
		AI: []port.CorpusDocument{
			{Path: "ai/m/business-01.md", Text: "z"},
			{Path: "ai/m/essay-01.md", Text: "w"},
		},
		GenreByID: map[string]string{"biz-1": "business", "note-1": "essay"},
	}
	c := GroupCorpus(files)
	if len(c[HumanWeb]) != 2 || len(c[HumanBusiness]) != 1 || len(c[AI]) != 2 || len(c[AIBusiness]) != 1 {
		t.Errorf("groups = web %d, hb %d, ai %d, ab %d", len(c[HumanWeb]), len(c[HumanBusiness]), len(c[AI]), len(c[AIBusiness]))
	}
	if c[HumanAozora][0].CharCount != 3 {
		t.Errorf("CharCount should count runes, got %d", c[HumanAozora][0].CharCount)
	}
	if len(c.Humans()) != 3 {
		t.Errorf("Humans = %d, want aozora+web", len(c.Humans()))
	}
}
