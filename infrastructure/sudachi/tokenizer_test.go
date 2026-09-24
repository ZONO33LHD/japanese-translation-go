package sudachi

import (
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/internal/testenv"
)

// 辞書がない環境ではスキップする（SudachiDict core は 200MB 超でリポジトリに含めない）。
func openTestTokenizer(t *testing.T) *Tokenizer {
	t.Helper()
	path := testenv.RequireDict(t)
	tk, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tk.Close() })
	return tk
}

func TestTokenizeUsesRuneOffsets(t *testing.T) {
	tk := openTestTokenizer(t)
	text := "「ＡＩ」は言えません。"
	ms, err := tk.Tokenize(text)
	if err != nil {
		t.Fatal(err)
	}
	rs := []rune(text)
	for _, m := range ms {
		if got := string(rs[m.Begin:m.End]); got != m.Surface {
			t.Errorf("rune slice [%d:%d] = %q, surface = %q", m.Begin, m.End, got, m.Surface)
		}
	}
	last := ms[len(ms)-1]
	if last.End != len(rs) {
		t.Errorf("last End = %d, want %d", last.End, len(rs))
	}
}

func TestTokenizeNegationNormalizedForm(t *testing.T) {
	tk := openTestTokenizer(t)
	ms, err := tk.Tokenize("言えません")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range ms {
		if m.Surface == "ん" && m.NormalizedForm == "ず" && m.POS1() == "助動詞" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ん to be normalized to ず: %+v", ms)
	}
}
