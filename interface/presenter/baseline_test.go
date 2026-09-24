package presenter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
)

func TestDecodeBaseline(t *testing.T) {
	cases := []struct {
		name         string
		data         string
		wantOK       bool
		wantErr      bool
		wantFindings int
		wantWarnings int
	}{
		{"not json", `{`, false, true, 0, 0},
		{"top-level array", `[]`, false, false, 0, 1},
		{"missing findings", `{"stats":{}}`, false, false, 0, 1},
		{"skips broken items", `{"findings":[{"category":"a","excerpt":"x"},1,{"category":2,"excerpt":"y"}]}`, true, false, 1, 1},
		{"empty findings", `{"findings":[]}`, true, false, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs, warnings, ok, err := DecodeBaseline([]byte(c.data))
			if (err != nil) != c.wantErr || ok != c.wantOK || len(fs) != c.wantFindings || len(warnings) != c.wantWarnings {
				t.Errorf("got findings=%d warnings=%v ok=%v err=%v", len(fs), warnings, ok, err)
			}
		})
	}
}

// lint --json の出力をそのまま --baseline に渡せることを、書き出しと読み戻しの往復で確かめる。
// キー名を片方だけ変えると、ここで壊れる。
func TestBaselineRoundTripThroughLintJSON(t *testing.T) {
	findings := []model.Finding{
		model.NewFinding(2, "forbidden_phrase", "重要なのは中身", model.SeverityInfo, "d", nil),
		model.NewFinding(5, "translationese", "することができる", model.SeverityInfo, "d", []int{5, 7}),
	}
	var buf bytes.Buffer
	if err := WriteLintJSON(&buf, LintReport{File: "a.md", Findings: findings}); err != nil {
		t.Fatal(err)
	}
	baseline, warnings, ok, err := DecodeBaseline(buf.Bytes())
	if err != nil || !ok || len(warnings) != 0 || len(baseline) != 2 {
		t.Fatalf("decode: baseline=%d warnings=%v ok=%v err=%v", len(baseline), warnings, ok, err)
	}
	marked, resolved, summary := lint.CompareBaseline(findings[:1], baseline)
	if summary != (lint.BaselineSummary{Resolved: 1, Persisting: 1}) || marked[0].Status != model.StatusPersisting {
		t.Fatalf("summary = %+v", summary)
	}

	buf.Reset()
	report := LintReport{File: "a.md", Findings: marked, Baseline: &BaselineReport{File: "prev.json", Summary: summary, Resolved: resolved}}
	if err := WriteLintJSON(&buf, report); err != nil {
		t.Fatal(err)
	}
	// 解消済みの finding は元の JSON のキー順のまま返す。
	if !strings.Contains(buf.String(), `"resolved": [
      {
        "line": 5,
        "category": "translationese",`) {
		t.Errorf("resolved finding not echoed verbatim:\n%s", buf.String())
	}
}
