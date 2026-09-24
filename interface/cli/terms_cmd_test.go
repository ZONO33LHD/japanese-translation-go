package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const termsDoc = "APIとは外部から機能を呼ぶ仕組みだ。\nAPIを使う。\n"

func TestTermsCommand(t *testing.T) {
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
			args:       []string{"terms", "d.md"},
			open:       openRuneTokenizer,
			wantCode:   ExitOK,
			wantPrefix: "=== terms: d.md ===\nhas_gloss_hint は",
			wantInOut:  []string{"L1 API (出現2回, 説明手掛かり: あり)", "    近傍: APIとは"},
		},
		{
			name:       "json",
			args:       []string{"terms", "--json", "d.md"},
			open:       openRuneTokenizer,
			wantCode:   ExitOK,
			wantPrefix: "{\n  \"terms\": [\n    {\n      \"term\": \"API\",\n      \"first_line\": 1,\n      \"count\": 2,\n      \"has_gloss_hint\": true,",
		},
		{name: "missing file", args: []string{"terms", "nope.md"}, open: openRuneTokenizer, wantCode: ExitInputError, wantErr: "ファイルが見つかりません"},
		{name: "no file arg", args: []string{"terms"}, open: openRuneTokenizer, wantCode: ExitUsage},
		{name: "tokenizer open failure", args: []string{"terms", "d.md"}, open: failingOpener, wantCode: ExitInputError, wantErr: "dictionary is not configured"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, out, errOut := newTestApp(memReader{"d.md": termsDoc}, c.open)
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

func TestTermsEmptyDocument(t *testing.T) {
	app, out, _ := newTestApp(memReader{"d.md": ""}, openRuneTokenizer)
	if code := app.Run(context.Background(), []string{"terms", "--json", "d.md"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var got map[string][]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["terms"] == nil || len(got["terms"]) != 0 {
		t.Errorf("empty document must yield terms: [] (not null): %s", out.String())
	}
}
