package presenter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/usecase/terms"
)

func TestTermsJSON(t *testing.T) {
	ts := []terms.Term{
		{Term: "API", FirstLine: 1, Count: 2, HasGlossHint: true, Context: "APIとは<仕組み>"},
		{Term: "リモートワーク", FirstLine: 4, Count: 1, Context: "リモートワークが広まった"},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, TermsJSON(ts)); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"terms\": [\n    {\n      \"term\": \"API\",\n      \"first_line\": 1,\n      \"count\": 2,\n" +
		"      \"has_gloss_hint\": true,\n      \"context\": \"APIとは<仕組み>\"\n    },"
	if !strings.HasPrefix(buf.String(), want) {
		t.Errorf("JSON prefix mismatch (HTML must not be escaped):\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "\"has_gloss_hint\": false,") {
		t.Errorf("false hint missing:\n%s", buf.String())
	}
}

func TestTermsJSONEmptyIsArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, TermsJSON(nil)); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "{\n  \"terms\": []\n}\n" {
		t.Errorf("got %q", buf.String())
	}
}

func TestWriteTermsHuman(t *testing.T) {
	cases := []struct {
		name string
		ts   []terms.Term
		want []string
	}{
		{"empty", nil, []string{"=== terms: d.md ===\n", "(用語候補なし)\n"}},
		{
			"with terms",
			[]terms.Term{
				{Term: "API", FirstLine: 1, Count: 2, HasGlossHint: true, Context: "APIとは"},
				{Term: "KV", FirstLine: 3, Count: 1, Context: "KVに移す"},
			},
			[]string{"L1 API (出現2回, 説明手掛かり: あり)\n    近傍: APIとは\n\n", "L3 KV (出現1回, 説明手掛かり: なし)\n    近傍: KVに移す\n"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteTermsHuman(&buf, "d.md", c.ts)
			for _, w := range c.want {
				if !strings.Contains(buf.String(), w) {
					t.Errorf("missing %q:\n%s", w, buf.String())
				}
			}
		})
	}
}

// 用語の近傍は複数行にまたがる。改行はそのまま表示し、制御文字だけを置き換える。
func TestWriteTermsHumanKeepsMultiLineContext(t *testing.T) {
	var buf bytes.Buffer
	WriteTermsHuman(&buf, "a.md", []terms.Term{
		{Term: "API", FirstLine: 1, Count: 1, Context: "# 見出し\n\nAPIとは\x1b[2J仕組み"},
	})
	out := buf.String()
	if !strings.Contains(out, "近傍: # 見出し\n\nAPIとは\\x1b[2J仕組み") {
		t.Errorf("output = %q", out)
	}
}
