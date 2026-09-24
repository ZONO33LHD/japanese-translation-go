package lint

import (
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

func TestCompareBaselineMultisetAndKeys(t *testing.T) {
	baseline := []BaselineFinding{
		{Category: "forbidden_phrase", Excerpt: "重要なのは 中身"},
		{Category: "forbidden_phrase", Excerpt: "重要なのは中身"},
		{Category: "low_burstiness", Excerpt: "burstiness=-0.500"},
		{Category: "translationese", Excerpt: "することができる"},
	}
	current := []model.Finding{
		{Line: 3, Category: "forbidden_phrase", Excerpt: "重要なのは中身"},
		{Line: 1, Category: "low_burstiness", Excerpt: "burstiness=-0.412"},
		{Line: 9, Category: "nominal_ending", Excerpt: "体言止め0件"},
	}
	marked, resolved, summary := CompareBaseline(current, baseline)
	want := []model.BaselineStatus{model.StatusPersisting, model.StatusPersisting, model.StatusNew}
	for i, f := range marked {
		if f.Status != want[i] {
			t.Errorf("marked[%d].Status = %s, want %s", i, f.Status, want[i])
		}
	}
	if current[0].Status != "" {
		t.Error("CompareBaseline must not mutate its input")
	}
	if summary != (BaselineSummary{Resolved: 2, New: 1, Persisting: 2}) || len(resolved) != 2 {
		t.Errorf("summary = %+v, resolved = %d", summary, len(resolved))
	}
}
