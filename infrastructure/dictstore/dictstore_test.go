package dictstore

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serve(t *testing.T, body []byte) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	return srv, hex.EncodeToString(sum[:])
}

func TestInstallExtractsDictionaryAndLicenses(t *testing.T) {
	archive := makeZip(t, map[string]string{
		"sudachi-dictionary-x/system_core.dic": "DIC",
		"sudachi-dictionary-x/LEGAL":           "legal",
		"sudachi-dictionary-x/LICENSE-2.0.txt": "apache",
		"../../evil/system_core.dic.bak":       "ignored",
		"sudachi-dictionary-x/other.txt":       "ignored",
	})
	srv, sum := serve(t, archive)
	s := Store{Dir: t.TempDir(), URL: srv.URL, SHA256: sum, Client: srv.Client()}
	got, err := s.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != s.Path() || !s.Installed() {
		t.Fatalf("path = %s, installed = %v", got, s.Installed())
	}
	entries, _ := os.ReadDir(s.Dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 3 {
		t.Errorf("only the dictionary and license files must be extracted: %v", names)
	}
	if b, _ := os.ReadFile(filepath.Join(s.Dir, "LEGAL")); string(b) != "legal" {
		t.Errorf("LEGAL = %q", b)
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	srv, _ := serve(t, makeZip(t, map[string]string{"system_core.dic": "DIC"}))
	s := Store{Dir: t.TempDir(), URL: srv.URL, SHA256: "00", Client: srv.Client()}
	if _, err := s.Install(context.Background()); err == nil {
		t.Fatal("checksum mismatch must fail")
	}
	if s.Installed() {
		t.Error("nothing must be extracted when the checksum does not match")
	}
}

func TestInstallFailsWithoutDictionary(t *testing.T) {
	srv, sum := serve(t, makeZip(t, map[string]string{"LEGAL": "legal"}))
	s := Store{Dir: t.TempDir(), URL: srv.URL, SHA256: sum, Client: srv.Client()}
	if _, err := s.Install(context.Background()); err == nil {
		t.Fatal("an archive without system_core.dic must fail")
	}
}

func TestInstallReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	s := Store{Dir: t.TempDir(), URL: srv.URL, SHA256: ArchiveSHA256, Client: srv.Client()}
	if _, err := s.Install(context.Background()); err == nil {
		t.Fatal("404 must fail")
	}
}

func TestDefaultUsesPinnedRelease(t *testing.T) {
	s, err := Default()
	if err != nil {
		t.Skip(err)
	}
	if s.URL != DefaultURL || s.SHA256 != ArchiveSHA256 || filepath.Base(s.Path()) != DictFile {
		t.Errorf("store = %+v", s)
	}
}
