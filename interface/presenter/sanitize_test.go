package presenter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

func TestSanitizeTerminal(t *testing.T) {
	cases := map[string]string{
		"普通の文\tタブ":               "普通の文\tタブ",
		"\x1b]52;c;ZXZpbA==\x07": `\x1b]52;c;ZXZpbA==\x07`,
		"a\x00b\x7fc\u009bd":     `a\x00b\x7fc\x9bd`,
		"改行\nも":                  "改行\nも",
		"a\u202Eb\u2066c\u200Fd": `a\u{202E}b\u{2066}c\u{200F}d`,
		"行\u2028区切り\u2029段落":     `行\u{2028}区切り\u{2029}段落`,
	}
	for in, want := range cases {
		if got := sanitizeTerminal(in); got != want {
			t.Errorf("sanitizeTerminal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteFindingHumanEscapesControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	WriteFindingHuman(&buf, model.Finding{Line: 1, Category: "forbidden_phrase", Severity: model.SeverityWarn,
		Excerpt: "重要なのは\x1b[2J", Detail: "detail\x1b]0;title\x07"})
	out := buf.String()
	if strings.ContainsRune(out, 0x1b) || strings.ContainsRune(out, 0x07) {
		t.Fatalf("raw control characters reached the terminal output: %q", out)
	}
	if !strings.Contains(out, `重要なのは\x1b[2J`) || !strings.Contains(out, `detail\x1b]0;title\x07`) {
		t.Errorf("output = %q", out)
	}
}

func TestFindingJSONKeepsOriginalText(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, FindingJSON(model.Finding{Excerpt: "a\x1bb"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"excerpt": "a\u001bb"`) {
		t.Errorf("JSON must keep the value and let encoding/json escape it: %s", buf.String())
	}
}

func TestTerminalSafeWriterEscapesAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewTerminalSafeWriter(&buf)
	// C1 制御文字 U+009B（CSI）は UTF-8 で 2 バイト。Write の境界で分割されても置き換えること。
	csi := []byte("\u009b")
	for _, chunk := range [][]byte{[]byte("前\x1b[31m"), csi[:1], csi[1:], []byte("後\n")} {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := buf.String(), `前\x1b[31m\x9b後`+"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if NewTerminalSafeWriter(w) != w {
		t.Error("wrapping twice must return the same writer")
	}
}
