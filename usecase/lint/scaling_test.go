package lint

import (
	"strings"
	"testing"
	"time"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

// 以下は 1 行（1 文）が長い入力で O(n²) に戻っていないかを見る。旧実装はこの規模で
// 数秒〜数分かかっていたので、1 秒（race 検出器ありは 10 秒）の上限で退行を見分けられる。
func scalingBudget() time.Duration {
	if raceEnabled {
		return 10 * time.Second
	}
	return time.Second
}

func within(t *testing.T, name string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if d := time.Since(start); d > scalingBudget() {
		t.Errorf("%s took %v (budget %v): quadratic behavior is back?", name, d, scalingBudget())
	}
}

func TestSurfaceDetectorsLinearOnLongLine(t *testing.T) {
	line := strings.Repeat("することができる。", 30000)
	lines, raw := linesOf(line)
	within(t, "translationese", func() { DetectTranslationese(lines, raw) })
	within(t, "sentence split", func() { splitForTest(lines, raw) })
}

// longSentence は 1 文に n 組の「これは〜」主語が並び、他動詞が一度も出ない形態素列を作る。
func longSentence(n int) model.TokenizedSentence {
	parts := make([]mo, 0, 3*n)
	for range n {
		parts = append(parts,
			mo{"これ", "代名詞", "*", "", ""},
			mo{"は", "助詞", "係助詞", "", ""},
			mo{"東京都庁舎新館", "名詞", "固有名詞", "", ""},
		)
	}
	return sent(1, parts...)
}

func TestMorphDetectorsLinearOnLongSentence(t *testing.T) {
	ts := []model.TokenizedSentence{longSentence(30000)}
	within(t, "inanimate_subject_morph", func() { DetectInanimateSubjectMorph(ts) })
	within(t, "reading load (kanji runs)", func() { DetectReadingLoad(ts, 90) })
}

func TestRelatedLinesAreCapped(t *testing.T) {
	var b strings.Builder
	for range 500 {
		b.WriteString("AではなくB。\n\n")
	}
	lines, raw := linesOf(b.String())
	fs := DetectAntithesisRepetition(lines, raw, DefaultParams())
	if len(fs) != 500 {
		t.Fatalf("got %d findings", len(fs))
	}
	if len(fs[0].RelatedLines) != maxRelatedLines || !strings.Contains(fs[0].Detail, "ほか480箇所") {
		t.Errorf("related = %d, detail = %q", len(fs[0].RelatedLines), fs[0].Detail)
	}
}

func TestExcerptSkipsMaskedSpanInsideSentence(t *testing.T) {
	ts := model.TokenizedSentence{
		Text:    "まず               を使う",
		RawText: "まず`kubectl apply`を使う",
	}
	// 形態素の窓が空白化された区間の途中（13 文字目）から始まっても、断片を出さない。
	if got := ts.RawSlice(13, 20); got != "を使う" {
		t.Errorf("RawSlice = %q, want %q", got, "を使う")
	}
	if got := ts.RawSlice(0, 2); got != "まず" {
		t.Errorf("RawSlice = %q", got)
	}
}

func TestExcerptSkipsMaskedSpanContainingSpaces(t *testing.T) {
	ts := model.TokenizedSentence{
		Text:    "前半       これは",
		RawText: "前半`x y z`これは",
	}
	for begin := 2; begin < 9; begin++ {
		if got := ts.RawSlice(begin, 12); got != "これは" {
			t.Errorf("RawSlice(%d, 12) = %q, want %q", begin, got, "これは")
		}
	}
}
