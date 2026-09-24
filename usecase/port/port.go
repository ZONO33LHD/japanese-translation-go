// Package port はユースケース層が外側（infrastructure）に要求するインターフェースを定義する。
// 依存性逆転により、ユースケースは具体的な形態素解析器や埋め込みモデルを知らない。
package port

import (
	"context"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

// Tokenizer は日本語テキストを分割単位 C の形態素列に分解する。
// 実装は複数 goroutine から同時に呼ばれてもよいこと。
type Tokenizer interface {
	Tokenize(text string) ([]model.Morpheme, error)
}

// Embedder は文の列を L2 正規化済みの埋め込みベクトル列に変換する。
type Embedder interface {
	Embed(ctx context.Context, sentences []string) ([][]float64, error)
	ModelName() string
}

// SourceReader は検査対象の文書を読み込む。
// 読み込めない場合は利用者向けのメッセージを持つ error を返す。
type SourceReader interface {
	Read(path string) (string, error)
}
