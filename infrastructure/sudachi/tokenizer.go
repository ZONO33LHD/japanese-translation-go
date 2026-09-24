// Package sudachi は sudachin-go を使って port.Tokenizer を実装する。
package sudachi

import (
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/ZONO33LHD/sudachin-go"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

// DictEnv はシステム辞書のパスを指定する環境変数名（sudachin CLI と共通）。
const DictEnv = "SUDACHIN_DICT"

// ErrDictNotConfigured は辞書のパスが与えられなかったときに返る。
var ErrDictNotConfigured = errors.New("sudachi system dictionary is not configured (use --dict or $" + DictEnv + ")")

// Tokenizer は sudachin.Analyzer を分割単位 C で呼び出すアダプタ。
type Tokenizer struct {
	analyzer *sudachin.Analyzer
}

// ResolveDictPath は明示指定を優先し、なければ環境変数から辞書パスを決める。
func ResolveDictPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if p := os.Getenv(DictEnv); p != "" {
		return p, nil
	}
	return "", ErrDictNotConfigured
}

// Open は dictPath の辞書を開く。
func Open(dictPath string) (*Tokenizer, error) {
	a, err := sudachin.Open(dictPath)
	if err != nil {
		return nil, fmt.Errorf("open sudachi dictionary %q: %w", dictPath, err)
	}
	return &Tokenizer{analyzer: a}, nil
}

// Close は辞書を解放する。
func (t *Tokenizer) Close() error { return t.analyzer.Close() }

// Tokenize は text を形態素に分割し、オフセットをルーン単位に変換して返す。
func (t *Tokenizer) Tokenize(text string) ([]model.Morpheme, error) {
	ms, err := t.analyzer.Analyze(text, sudachin.ModeC)
	if err != nil {
		return nil, fmt.Errorf("tokenize: %w", err)
	}
	out := make([]model.Morpheme, len(ms))
	// Begin/End は昇順に並ぶので、バイト→ルーンの変換は 1 回の走査で済ませる。
	byteCursor, runeCursor := 0, 0
	toRune := func(b int) int {
		if b < byteCursor {
			return utf8.RuneCountInString(text[:b])
		}
		runeCursor += utf8.RuneCountInString(text[byteCursor:b])
		byteCursor = b
		return runeCursor
	}
	for i, m := range ms {
		begin := toRune(m.Begin)
		end := toRune(m.End)
		out[i] = model.Morpheme{
			Surface:        m.Surface,
			POS:            [6]string(m.POS),
			DictionaryForm: m.DictionaryForm,
			NormalizedForm: m.NormalizedForm,
			ReadingForm:    m.ReadingForm,
			Begin:          begin,
			End:            end,
		}
	}
	return out, nil
}
