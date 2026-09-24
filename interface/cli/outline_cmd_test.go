package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const outlineDoc = "# 設計メモ\n\n最初の段落です。二文目。\n\n- 項目A\n- 項目B\n\n## まとめ\n"

func TestOutlineCommand(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		open       TokenizerOpener
		wantCode   int
		wantPrefix string
		wantInOut  []string
		wantErr    string
	}{
		{
			name:       "human",
			args:       []string{"outline", "d.md"},
			open:       openRuneTokenizer,
			wantCode:   ExitOK,
			wantPrefix: "=== outline: d.md ===\n",
			wantInOut:  []string{"    L1  # 設計メモ", "    L3    最初の段落です。", "    L5    (箇条書き 2 項目)", "=== 見出し統計", "見出し総数: 2"},
		},
		{
			name:       "json",
			args:       []string{"outline", "--json", "d.md"},
			open:       openRuneTokenizer,
			wantCode:   ExitOK,
			wantPrefix: "{\n  \"outline\": [\n    {\n      \"line\": 1,\n      \"kind\": \"heading\",\n      \"level\": 1,",
			wantInOut:  []string{`"heading_stats": {`, `"total_headings": 2`, `"level": null`},
		},
		{name: "missing file", args: []string{"outline", "nope.md"}, open: openRuneTokenizer, wantCode: ExitInputError, wantErr: "ファイルが見つかりません"},
		{name: "no file arg", args: []string{"outline", "--json"}, open: openRuneTokenizer, wantCode: ExitUsage},
		{name: "tokenizer open failure", args: []string{"outline", "d.md"}, open: failingOpener, wantCode: ExitInputError, wantErr: "dictionary is not configured"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, out, errOut := newTestApp(memReader{"d.md": outlineDoc}, c.open)
			if code := app.Run(context.Background(), c.args); code != c.wantCode {
				t.Fatalf("exit = %d, want %d (stderr=%q)", code, c.wantCode, errOut.String())
			}
			if !strings.HasPrefix(out.String(), c.wantPrefix) {
				t.Errorf("stdout prefix mismatch:\n%s", out.String())
			}
			for _, s := range c.wantInOut {
				if !strings.Contains(out.String(), s) {
					t.Errorf("stdout missing %q:\n%s", s, out.String())
				}
			}
			if c.wantErr != "" && !strings.Contains(errOut.String(), c.wantErr) {
				t.Errorf("stderr = %q, want to contain %q", errOut.String(), c.wantErr)
			}
		})
	}
}

func TestOutlineJSONIsValid(t *testing.T) {
	app, out, _ := newTestApp(memReader{"d.md": outlineDoc}, openRuneTokenizer)
	if code := app.Run(context.Background(), []string{"outline", "d.md", "--json"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var got struct {
		Outline []struct {
			Line int    `json:"line"`
			Kind string `json:"kind"`
		} `json:"outline"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, len(got.Outline))
	for i, e := range got.Outline {
		kinds[i] = e.Kind
	}
	if strings.Join(kinds, ",") != "heading,lead,bullets,heading" {
		t.Errorf("kinds = %v", kinds)
	}
}
