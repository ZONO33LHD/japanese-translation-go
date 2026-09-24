package cli

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// memReader はパス→本文のマップから読む SourceReader。
type memReader map[string]string

func (m memReader) Read(path string) (string, error) {
	s, ok := m[path]
	if !ok {
		return "", fmt.Errorf("エラー: ファイルが見つかりません: %s", path)
	}
	return s, nil
}

// runeTokenizer は 1 文字を 1 名詞として返す。CLI の配線だけを確かめるので形態素の質は問わない。
type runeTokenizer struct{}

func (runeTokenizer) Tokenize(text string) ([]model.Morpheme, error) {
	var ms []model.Morpheme
	for i, r := range []rune(text) {
		ms = append(ms, model.Morpheme{Surface: string(r), POS: [6]string{"名詞", "普通名詞"}, DictionaryForm: string(r), Begin: i, End: i + 1})
	}
	return ms, nil
}

func openRuneTokenizer(string) (port.Tokenizer, func() error, error) {
	return runeTokenizer{}, func() error { return nil }, nil
}

func failingOpener(string) (port.Tokenizer, func() error, error) {
	return nil, nil, errors.New("sudachi system dictionary is not configured")
}

// newTestApp は tokenizer の開き方を差し替えられるテスト用 App を作る。
func newTestApp(files memReader, open TokenizerOpener) (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &App{Reader: files, OpenTokenizer: open, Stdout: &out, Stderr: &errOut}, &out, &errOut
}
