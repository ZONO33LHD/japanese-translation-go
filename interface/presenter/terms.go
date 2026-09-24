package presenter

import (
	"fmt"
	"io"

	"github.com/ZONO33LHD/japanese-translation-go/usecase/terms"
)

// TermsJSON は terms サブコマンドの JSON 出力を組み立てる。
func TermsJSON(ts []terms.Term) Object {
	out := make([]Object, len(ts))
	for i, t := range ts {
		out[i] = Object{
			{"term", t.Term},
			{"first_line", t.FirstLine},
			{"count", t.Count},
			{"has_gloss_hint", t.HasGlossHint},
			{"context", t.Context},
		}
	}
	return Object{{"terms", out}}
}

// WriteTermsHuman は用語候補を人間可読形式で書き出す。
func WriteTermsHuman(w io.Writer, path string, ts []terms.Term) {
	fmt.Fprintf(w, "=== terms: %s ===\n", sanitizeTerminal(path))
	fmt.Fprintln(w, "has_gloss_hint は「説明済みと判定した」印ではなく、初出近傍に説明マーカーが"+
		"見つかったという機械的なヒントに過ぎない。要確認は人間/AIの判断に委ねる。")
	fmt.Fprintln(w)
	if len(ts) == 0 {
		fmt.Fprintln(w, "(用語候補なし)")
		return
	}
	for _, t := range ts {
		hint := "なし"
		if t.HasGlossHint {
			hint = "あり"
		}
		fmt.Fprintf(w, "L%d %s (出現%d回, 説明手掛かり: %s)\n", t.FirstLine, sanitizeTerminal(t.Term), t.Count, hint)
		fmt.Fprintf(w, "    近傍: %s\n\n", sanitizeTerminal(t.Context))
	}
}
