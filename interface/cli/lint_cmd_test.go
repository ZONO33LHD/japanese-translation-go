package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newLintApp(files memReader) (*App, *bytes.Buffer, *bytes.Buffer) {
	return newTestApp(files, openRuneTokenizer)
}

const lintDoc = "重要なのは中身です。\n\nこのように考えると分かりやすい。\n"

func TestLintJSONTopLevelKeys(t *testing.T) {
	app, out, _ := newLintApp(memReader{"d.md": lintDoc})
	if code := app.Run(context.Background(), []string{"lint", "d.md", "--json", "--reading-load"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"file", "stats", "findings", "reading_load"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	if _, ok := got["baseline"]; ok {
		t.Error("baseline section must be absent without --baseline")
	}
	if !strings.HasPrefix(out.String(), "{\n  \"file\": \"d.md\",\n  \"stats\": {\n    \"total_findings\": 2,") {
		t.Errorf("key order differs from the original:\n%s", out.String()[:120])
	}
}

func TestLintBaselineRoundTrip(t *testing.T) {
	app, out, _ := newLintApp(memReader{"d.md": lintDoc})
	if code := app.Run(context.Background(), []string{"lint", "--json", "d.md"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	prev := "prev.json"
	prevJSON := out.String()

	app, out, _ = newLintApp(memReader{"d.md": "このように考えると分かりやすい。\n", prev: prevJSON})
	if code := app.Run(context.Background(), []string{"lint", "d.md", "--baseline", prev}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "ベースライン比較: 解消: 1件 / 新規: 0件 / 継続: 1件") {
		t.Errorf("unexpected report:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "[継続] [情報] L1 (forbidden_phrase)") {
		t.Errorf("status tag missing:\n%s", out.String())
	}
}

func TestLintInputErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing file", []string{"lint", "nope.md"}, ExitInputError},
		{"missing baseline", []string{"lint", "d.md", "--baseline", filepath.Join(t.TempDir(), "none.json")}, ExitInputError},
		{"bad genre", []string{"lint", "d.md", "--genre", "poem"}, ExitUsage},
		{"no file", []string{"lint", "--json"}, ExitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, _, _ := newLintApp(memReader{"d.md": lintDoc})
			if code := app.Run(context.Background(), c.args); code != c.want {
				t.Errorf("exit = %d, want %d", code, c.want)
			}
		})
	}
}

func TestLintBrokenBaselineFallsBackWithWarning(t *testing.T) {
	prev := "prev.json"
	app, out, errOut := newLintApp(memReader{"d.md": lintDoc, prev: `[1, 2]`})
	if code := app.Run(context.Background(), []string{"lint", "d.md", "--baseline", prev}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut.String(), "警告: --baseline の内容が JSON オブジェクトではありません") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if strings.Contains(out.String(), "ベースライン比較") {
		t.Error("baseline summary must not be printed after fallback")
	}
}

// --baseline は文書とは別の Reader（上限の広いもの）で読む。
func TestLintBaselineUsesBaselineReader(t *testing.T) {
	app, out, _ := newLintApp(memReader{"d.md": lintDoc})
	if code := app.Run(context.Background(), []string{"lint", "--json", "d.md"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	prev := out.String()

	app, out, errOut := newLintApp(memReader{"d.md": lintDoc})
	app.BaselineReader = memReader{"big.json": prev}
	if code := app.Run(context.Background(), []string{"lint", "d.md", "--baseline", "big.json"}); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out.String(), "ベースライン比較: 解消: 0件 / 新規: 0件 / 継続: 2件") {
		t.Errorf("report = %s", out)
	}
}

func TestLintBaselineErrorHasSinglePrefix(t *testing.T) {
	app, _, errOut := newLintApp(memReader{"d.md": lintDoc})
	if code := app.Run(context.Background(), []string{"lint", "d.md", "--baseline", "none.json"}); code != ExitInputError {
		t.Fatalf("exit = %d", code)
	}
	if got := errOut.String(); !strings.HasPrefix(got, "エラー: --baseline を読み込めません: ファイルが見つかりません: none.json") {
		t.Errorf("stderr = %q", got)
	}
}
