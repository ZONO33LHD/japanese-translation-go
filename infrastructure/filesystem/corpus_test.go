package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCorpusLoaderLoad(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "human/aozora/b.md"), "md本文")
	writeFile(t, filepath.Join(dir, "human/aozora/a.txt"), "txt本文\r\n2行目")
	writeFile(t, filepath.Join(dir, "human/aozora/blank.txt"), " \n　")
	writeFile(t, filepath.Join(dir, "human/aozora/bin.txt"), "\xff\xfe")
	writeFile(t, filepath.Join(dir, "human/aozora/.gitkeep"), "")
	writeFile(t, filepath.Join(dir, "ai/m1/business-01.md"), "AI文")
	writeFile(t, filepath.Join(dir, "sources.json"), `[{"id":"biz-1","genre":"business"},{"genre":"x"},"bad"]`)

	files, err := CorpusLoader{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Aozora) != 2 {
		t.Fatalf("aozora = %d docs, want 2 (blank and invalid UTF-8 skipped)", len(files.Aozora))
	}
	// .txt を先に、.md を後に並べる（元実装のパターン順）。
	if filepath.Base(files.Aozora[0].Path) != "a.txt" || filepath.Base(files.Aozora[1].Path) != "b.md" {
		t.Errorf("order = %s, %s", files.Aozora[0].Path, files.Aozora[1].Path)
	}
	if files.Aozora[0].Text != "txt本文\n2行目" {
		t.Errorf("CRLF should be normalized, got %q", files.Aozora[0].Text)
	}
	if len(files.Web) != 0 {
		t.Errorf("missing web dir should yield no docs, got %d", len(files.Web))
	}
	if len(files.AI) != 1 {
		t.Errorf("ai = %d docs", len(files.AI))
	}
	if files.GenreByID["biz-1"] != "business" || len(files.GenreByID) != 1 {
		t.Errorf("genre map = %v", files.GenreByID)
	}
}

func TestCorpusLoaderBrokenSourcesJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sources.json"), "{not json")
	files, err := CorpusLoader{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(files.GenreByID) != 0 {
		t.Errorf("broken sources.json should yield empty map, got %v", files.GenreByID)
	}
}

func TestCorpusLoaderMissingDirectoryIsError(t *testing.T) {
	if _, err := (CorpusLoader{Dir: filepath.Join(t.TempDir(), "none")}).Load(); err == nil {
		t.Fatal("missing corpus directory must be an error, not an empty corpus")
	}
}

func TestCorpusLoaderRecordsSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ai", "ok.md"), "本文です。")
	writeFile(t, filepath.Join(dir, "ai", "bad.md"), "\xff\xfe")
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeFile(t, outside, "外部の文書。")
	if err := os.Symlink(outside, filepath.Join(dir, "ai", "link.md")); err != nil {
		t.Skip("symlink not supported:", err)
	}
	files, err := CorpusLoader{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if files.Total() != 1 || len(files.Skipped) != 2 {
		t.Fatalf("total=%d skipped=%q", files.Total(), files.Skipped)
	}
}

func TestCorpusLoaderReportsSymlinkedSubdirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "human", "web", "h.md"), "人間の文書。")
	real := t.TempDir()
	writeFile(t, filepath.Join(real, "a.md"), "AI の文書。")
	if err := os.Symlink(real, filepath.Join(dir, "ai")); err != nil {
		t.Skip("symlink not supported:", err)
	}
	files, err := CorpusLoader{Dir: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(files.AI) != 0 || len(files.Skipped) != 1 || !strings.Contains(files.Skipped[0], "シンボリックリンクのディレクトリは辿りません") {
		t.Fatalf("ai=%d skipped=%q", len(files.AI), files.Skipped)
	}
}
