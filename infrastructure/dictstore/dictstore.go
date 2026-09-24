// Package dictstore は SudachiDict core を取得してユーザーのキャッシュディレクトリに置く。
//
// 辞書は Works Applications の SudachiDict（Apache License 2.0）で、このリポジトリの
// GitHub Releases に公式配布の zip を改変せず再配布している。取得した zip は固定の
// SHA-256 と照合してから展開し、ライセンス表示（LICENSE-2.0.txt・LEGAL）も一緒に置く。
package dictstore

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

const (
	// Version は検出件数の回帰テストで確認した辞書の版。
	Version = "20260723"
	// ArchiveSHA256 は再配布している zip の SHA-256（公式配布物と同一）。
	ArchiveSHA256 = "b6e835f63440f97474c2da45d80950f73746e632e40bbfc168b4041729135e1f"
	archiveName   = "sudachi-dictionary-" + Version + "-core.zip"
	// DefaultURL は再配布先。公式配布サーバーではなく自前の Release から取る。
	DefaultURL = "https://github.com/ZONO33LHD/japanese-translation-go/releases/download/sudachidict-core-" + Version + "/" + archiveName
	// DictFile は展開後のシステム辞書のファイル名。
	DictFile = "system_core.dic"

	// maxArchiveBytes は zip の上限。公式の zip は約 72 MB。
	maxArchiveBytes = 128 << 20
	// maxEntryBytes は展開する 1 ファイルの上限。辞書本体は約 217 MB。
	maxEntryBytes   = 512 << 20
	downloadTimeout = 10 * time.Minute
)

// 展開するファイル。zip 内のパスは使わず名前だけで照合する（パストラバーサル対策）。
var extractNames = map[string]bool{DictFile: true, "LICENSE-2.0.txt": true, "LEGAL": true}

// Store は辞書の置き場所と取得元。
type Store struct {
	Dir    string
	URL    string
	SHA256 string
	Client *http.Client
}

// Default はユーザーのキャッシュディレクトリ（例: ~/Library/Caches/natural-japanese/...）を使う Store を返す。
func Default() (Store, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return Store{}, fmt.Errorf("resolve user cache directory: %w", err)
	}
	return Store{
		Dir:    filepath.Join(base, "natural-japanese", "sudachidict-core-"+Version),
		URL:    DefaultURL,
		SHA256: ArchiveSHA256,
	}, nil
}

// Path は展開済みの辞書ファイルのパス。
func (s Store) Path() string { return filepath.Join(s.Dir, DictFile) }

// Installed は辞書が展開済みかを返す。
func (s Store) Installed() bool {
	info, err := os.Stat(s.Path())
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// Install は zip を取得し、SHA-256 を照合してから辞書とライセンス表示を Dir に展開する。
// 途中で失敗しても Dir に壊れた辞書が残らないよう、一時ファイルに書いてから rename する。
func (s Store) Install(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("create dictionary directory: %w", err)
	}
	archive, err := os.CreateTemp(s.Dir, "download-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		_ = archive.Close()
		_ = os.Remove(archive.Name())
	}()

	size, err := s.download(ctx, archive)
	if err != nil {
		return "", err
	}
	if err := verifySHA256(archive, s.SHA256); err != nil {
		return "", err
	}
	if err := extract(archive, size, s.Dir); err != nil {
		return "", err
	}
	if !s.Installed() {
		return "", fmt.Errorf("archive does not contain %s", DictFile)
	}
	return s.Path(), nil
}

func (s Store) download(ctx context.Context, dst *os.File) (int64, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download dictionary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download dictionary: status %d", resp.StatusCode)
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return 0, fmt.Errorf("download dictionary: %w", err)
	}
	if n > maxArchiveBytes {
		return 0, fmt.Errorf("dictionary archive exceeds %d MiB", maxArchiveBytes>>20)
	}
	return n, nil
}

func verifySHA256(f *os.File, want string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind archive: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("dictionary checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

func extract(f *os.File, size int64, dir string) error {
	zr, err := zip.NewReader(f, size)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	for _, entry := range zr.File {
		name := path.Base(entry.Name)
		if entry.FileInfo().IsDir() || !extractNames[name] {
			continue
		}
		if err := extractEntry(entry, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(entry *zip.File, dst string) error {
	rc, err := entry.Open()
	if err != nil {
		return fmt.Errorf("open %s in archive: %w", entry.Name, err)
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".extract-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	n, err := io.Copy(tmp, io.LimitReader(rc, maxEntryBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("extract %s: %w", entry.Name, err)
	}
	if n > maxEntryBytes {
		return errors.New("archive entry " + entry.Name + " is too large")
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return fmt.Errorf("install %s: %w", dst, err)
	}
	return nil
}
