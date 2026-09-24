package dictstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// 再配布している実物の zip で、照合と展開が通ることを確かめる。
// 72 MB あるので、zip のパスが $NJ_DICT_ARCHIVE で渡されたとき（CI など）だけ実行する。
func TestInstallRealArchive(t *testing.T) {
	archivePath := os.Getenv("NJ_DICT_ARCHIVE")
	if archivePath == "" {
		t.Skip("$NJ_DICT_ARCHIVE is not set")
	}
	b, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(b) }))
	t.Cleanup(srv.Close)
	s := Store{Dir: t.TempDir(), URL: srv.URL, SHA256: ArchiveSHA256, Client: srv.Client()}
	if _, err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{DictFile, "LEGAL", "LICENSE-2.0.txt"} {
		if info, err := os.Stat(s.Dir + "/" + name); err != nil || info.Size() == 0 {
			t.Errorf("%s was not extracted: %v", name, err)
		}
	}
}
