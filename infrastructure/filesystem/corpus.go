package filesystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// CorpusLoader は corpus/ ディレクトリ（human/aozora, human/web, ai, sources.json）を読む。
type CorpusLoader struct {
	Dir string
}

// Load はコーパスを読み込む。サブディレクトリが欠けていても読めた分だけ返すが、
// 読み飛ばしたファイルは Skipped に理由付きで残し、黙って捨てない。
func (l CorpusLoader) Load() (port.CorpusFiles, error) {
	info, err := os.Stat(l.Dir)
	if err != nil {
		return port.CorpusFiles{}, fmt.Errorf("エラー: コーパスのディレクトリを開けません: %s (%w)", l.Dir, err)
	}
	if !info.IsDir() {
		return port.CorpusFiles{}, fmt.Errorf("エラー: コーパスにはディレクトリを指定してください: %s", l.Dir)
	}
	var skipped []string
	aozora, err := readTexts(filepath.Join(l.Dir, "human", "aozora"), []string{".txt", ".md"}, &skipped)
	if err != nil {
		return port.CorpusFiles{}, err
	}
	web, err := readTexts(filepath.Join(l.Dir, "human", "web"), []string{".md", ".txt"}, &skipped)
	if err != nil {
		return port.CorpusFiles{}, err
	}
	ai, err := readTexts(filepath.Join(l.Dir, "ai"), []string{".md", ".txt"}, &skipped)
	if err != nil {
		return port.CorpusFiles{}, err
	}
	genres, genreWarn := loadGenreMap(filepath.Join(l.Dir, "sources.json"))
	if genreWarn != "" {
		skipped = append(skipped, genreWarn)
	}
	return port.CorpusFiles{Aozora: aozora, Web: web, AI: ai, GenreByID: genres, Skipped: skipped}, nil
}

// readTexts は拡張子ごとに再帰的に集めてパス順に並べ、拡張子の指定順に連結する（元実装の rglob と同じ並び）。
// シンボリックリンクは辿らない。コーパス外の巨大なファイルや FIFO を読みに行かないため。
func readTexts(dir string, exts []string, skipped *[]string) ([]port.CorpusDocument, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err == nil && info.Mode()&fs.ModeSymlink != 0 {
		// WalkDir は起点のシンボリックリンクを辿らず 0 件になるので、黙って空にせず知らせる。
		*skipped = append(*skipped, "シンボリックリンクのディレクトリは辿りません: "+dir)
		return nil, nil
	}
	byExt := map[string][]string{}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			*skipped = append(*skipped, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if !slices.Contains(exts, ext) {
			return nil
		}
		if !d.Type().IsRegular() {
			*skipped = append(*skipped, path+": 通常ファイルではない（シンボリックリンク等）")
			return nil
		}
		byExt[ext] = append(byExt[ext], path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk corpus directory %s: %w", dir, err)
	}
	var docs []port.CorpusDocument
	for _, ext := range exts {
		paths := byExt[ext]
		slices.Sort(paths)
		for _, p := range paths {
			text, reason := readCorpusFile(p)
			if reason != "" {
				*skipped = append(*skipped, p+": "+reason)
				continue
			}
			if pystr.IsBlank(text) {
				continue
			}
			docs = append(docs, port.CorpusDocument{Path: p, Text: text})
		}
	}
	return docs, nil
}

func readCorpusFile(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "", err.Error()
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSourceBytes+1))
	if err != nil {
		return "", err.Error()
	}
	if int64(len(b)) > maxSourceBytes {
		return "", fmt.Sprintf("上限 %d MiB を超えている", maxSourceBytes>>20)
	}
	if !utf8.Valid(b) {
		return "", "UTF-8 ではない"
	}
	return normalizeNewlines(string(b)), ""
}

// loadGenreMap は sources.json の id → genre を読む。無ければ空、壊れていれば警告文も返す。
func loadGenreMap(path string) (map[string]string, string) {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, ""
	}
	if err != nil {
		return out, fmt.Sprintf("%s: %v", path, err)
	}
	var entries []any
	if err := json.Unmarshal(b, &entries); err != nil {
		return out, fmt.Sprintf("%s: JSON として読めない (%v)", path, err)
	}
	for _, e := range entries {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		genre, _ := m["genre"].(string)
		out[id] = genre
	}
	return out, ""
}
