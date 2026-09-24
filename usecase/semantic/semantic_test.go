package semantic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

// fakeEmbedder は文の並び順に応じた 2 次元の単位ベクトル（角度 angles[i]）を返す。
type fakeEmbedder struct {
	angles []float64
	scale  float64
	calls  int
	err    error
}

func (f *fakeEmbedder) Embed(_ context.Context, sentences []string) ([][]float64, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	scale := f.scale
	if scale == 0 {
		scale = 1
	}
	out := make([][]float64, len(sentences))
	for i := range sentences {
		if f.angles == nil {
			out[i] = make([]float64, len(sentences))
			out[i][i] = scale
			continue
		}
		a := f.angles[i]
		// 正規化されていないベクトルでも同じ結果になることを確かめるため scale を掛ける。
		out[i] = []float64{scale * math.Cos(a), scale * math.Sin(a)}
	}
	return out, nil
}

func (f *fakeEmbedder) ModelName() string { return "fake-model" }

func doc(n int) string {
	var b strings.Builder
	b.WriteString("# 見出しは文に含めない\n\n")
	for i := range n {
		fmt.Fprintf(&b, "これは%d番目の文です。\n", i)
	}
	return b.String()
}

func constantStep(n int, step float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(i) * step
	}
	return out
}

// alternatingStep は隣接類似度が 1 と cos(d) を交互にとる角度列（range = 1-cos(d)）。
func alternatingStep(n int, d float64) []float64 {
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		out[i] = out[i-1]
		if i%2 == 0 {
			out[i] += d
		}
	}
	return out
}

func categories(fs []model.Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Category
	}
	return out
}

func TestRunSemanticSkipsShortDocumentWithoutCallingEmbedder(t *testing.T) {
	emb := &fakeEmbedder{}
	fs, st, err := RunSemantic(context.Background(), doc(9), model.GenreNone, emb)
	if err != nil {
		t.Fatal(err)
	}
	if emb.calls != 0 {
		t.Errorf("embedder called %d times, want 0", emb.calls)
	}
	if len(fs) != 0 || !st.Skipped || st.Metrics != nil {
		t.Errorf("unexpected result: findings=%v stats=%+v", fs, st)
	}
	want := "文数が9文と少なく（目安10文未満）、統計的な起伏・反復の測定が安定しないため意味的検出をスキップしました。"
	if st.SkipReason != want {
		t.Errorf("SkipReason = %q, want %q", st.SkipReason, want)
	}
	if st.Model != "fake-model" || st.FlatnessThreshold != DefaultFlatnessThreshold {
		t.Errorf("stats = %+v", st)
	}
}

func TestRunSemanticFlatDocumentFiresFlatnessAndTopicJump(t *testing.T) {
	emb := &fakeEmbedder{angles: constantStep(10, 0.1), scale: 3}
	fs, st, err := RunSemantic(context.Background(), doc(10), model.GenreNone, emb)
	if err != nil {
		t.Fatal(err)
	}
	if st.NSentences != 10 || st.Skipped {
		t.Fatalf("stats = %+v", st)
	}
	got := categories(fs)
	want := []string{categoryTopicFlatness, categoryTopicJumpMin}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	if fs[0].Line != 3 {
		t.Errorf("line = %d, want 3 (first sentence after the heading)", fs[0].Line)
	}
	if fs[0].Excerpt != "隣接文類似度レンジ=0.0000（閾値0.1612以下）" {
		t.Errorf("flatness excerpt = %q", fs[0].Excerpt)
	}
	if fs[0].Severity != model.SeverityWarn {
		t.Errorf("flatness severity = %s", fs[0].Severity)
	}
	wantJump := fmt.Sprintf("隣接文類似度最小値=%.4f（参考閾値0.7658以上）", math.Cos(0.1))
	if fs[1].Excerpt != wantJump {
		t.Errorf("topic jump excerpt = %q, want %q", fs[1].Excerpt, wantJump)
	}
}

func TestRunSemanticFlatnessThresholdDependsOnGenre(t *testing.T) {
	// range = 1 - cos(d) = 0.158: 共通閾値・essay 閾値以下だが business・tech 閾値より大きい。
	d := math.Acos(1 - 0.158)
	cases := []struct {
		genre model.Genre
		fires bool
	}{
		{model.GenreNone, true},
		{model.GenreEssay, true},
		{model.GenreBusiness, false},
		{model.GenreTech, false},
	}
	for _, c := range cases {
		emb := &fakeEmbedder{angles: alternatingStep(12, d)}
		fs, st, err := RunSemantic(context.Background(), doc(12), c.genre, emb)
		if err != nil {
			t.Fatal(err)
		}
		fired := false
		for _, f := range fs {
			if f.Category == categoryTopicFlatness {
				fired = true
			}
		}
		if fired != c.fires {
			t.Errorf("genre %q: flatness fired=%v, want %v (threshold %v)", c.genre, fired, c.fires, st.FlatnessThreshold)
		}
	}
}

func TestRunSemanticSecondaryRepetitionMax(t *testing.T) {
	// 全文が互いに直交: 言い換え反復が皆無（=AI寄りの参考指標が発火）、隣接の起伏もゼロ。
	emb := &fakeEmbedder{scale: 2}
	fs, _, err := RunSemantic(context.Background(), doc(10), model.GenreNone, emb)
	if err != nil {
		t.Fatal(err)
	}
	got := categories(fs)
	want := []string{categoryTopicFlatness, categorySemanticRepetitionMax}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("categories = %v, want %v", got, want)
	}
	if fs[1].Excerpt != "非隣接文ペア類似度max=0.0000（参考閾値0.9322以下）" {
		t.Errorf("excerpt = %q", fs[1].Excerpt)
	}
	if fs[1].Severity != model.SeverityInfo {
		t.Errorf("severity = %s", fs[1].Severity)
	}
}

func TestRunSemanticPropagatesEmbedderError(t *testing.T) {
	emb := &fakeEmbedder{err: errors.New("boom")}
	_, _, err := RunSemantic(context.Background(), doc(10), model.GenreNone, emb)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}

func TestComputeMetrics(t *testing.T) {
	if m := ComputeMetrics([][]float64{{1, 0}, {0, 1}}); m.CoherenceFlatnessRange != nil || m.TopicJumpMin != nil || m.SemanticRepetitionMax != nil {
		t.Errorf("n<3 should yield all-nil metrics: %+v", m)
	}
	// n==3: 隣接 (0,1)=0, (1,2)=0.6 / 非隣接 (0,2)=0.8
	m := ComputeMetrics([][]float64{{1, 0}, {0, 2}, {0.8, 0.6}})
	approx := func(p *float64, want float64) bool { return p != nil && math.Abs(*p-want) < 1e-12 }
	if !approx(m.CoherenceFlatnessRange, 0.6) || !approx(m.TopicJumpMin, 0) || !approx(m.SemanticRepetitionMax, 0.8) {
		t.Errorf("metrics = range %v, jump %v, rep %v", *m.CoherenceFlatnessRange, *m.TopicJumpMin, *m.SemanticRepetitionMax)
	}
}
