package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSourceFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSourceReaderErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		path string
		want string
	}{
		{"not found", filepath.Join(dir, "none.md"), "ファイルが見つかりません"},
		{"directory", dir, "ディレクトリが指定されました"},
		{"invalid utf-8", writeSourceFile(t, "bad.md", []byte{0xff, 0xfe, 'a'}), "invalid UTF-8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := SourceReader{}.Read(c.path)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

func TestSourceReaderRejectsTooLargeFile(t *testing.T) {
	old := maxSourceBytes
	maxSourceBytes = 8
	t.Cleanup(func() { maxSourceBytes = old })
	p := writeSourceFile(t, "big.md", []byte("123456789"))
	if _, err := (SourceReader{}).Read(p); err == nil || !strings.Contains(err.Error(), "大きすぎます") {
		t.Errorf("err = %v", err)
	}
	p = writeSourceFile(t, "ok.md", []byte("12345678"))
	if _, err := (SourceReader{}).Read(p); err != nil {
		t.Errorf("file at the limit must be accepted: %v", err)
	}
}

func TestSourceReaderNormalizesNewlines(t *testing.T) {
	p := writeSourceFile(t, "crlf.md", []byte("a\r\nb\rc\n"))
	got, err := SourceReader{}.Read(p)
	if err != nil || got != "a\nb\nc\n" {
		t.Errorf("Read = %q, %v", got, err)
	}
}

func TestSourceReaderMaxBytesOverride(t *testing.T) {
	path := writeSourceFile(t, "big.json", []byte("0123456789"))
	if _, err := (SourceReader{MaxBytes: 4}).Read(path); err == nil || !strings.Contains(err.Error(), "大きすぎます") {
		t.Fatalf("MaxBytes=4 should reject a 10-byte file: %v", err)
	}
	if got, err := (SourceReader{MaxBytes: 64}).Read(path); err != nil || got != "0123456789" {
		t.Fatalf("MaxBytes=64: %q, %v", got, err)
	}
}
