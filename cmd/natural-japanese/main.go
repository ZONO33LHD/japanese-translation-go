// Command natural-japanese は仕事の日本語文書の「AI臭さ」と読解負荷を機械的に検出する CLI。
//
// このファイルはコンポジションルートで、infrastructure の具象実装を組み立てて
// interface/cli に注入するだけの責務を持つ。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/embedding"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/filesystem"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/sudachi"
	"github.com/ZONO33LHD/japanese-translation-go/interface/cli"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := newApp().Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

func newApp() *cli.App {
	return &cli.App{
		Reader:         filesystem.SourceReader{},
		BaselineReader: filesystem.SourceReader{MaxBytes: filesystem.BaselineMaxBytes},
		OpenTokenizer:  openTokenizer,
		NewEmbedder:    newEmbedder,
		NewCorpusLoader: func(dir string) port.CorpusLoader {
			return filesystem.CorpusLoader{Dir: dir}
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func openTokenizer(dictPath string) (port.Tokenizer, func() error, error) {
	path, err := sudachi.ResolveDictPath(dictPath)
	if err != nil {
		return nil, nil, fmt.Errorf("エラー: 辞書が指定されていません（--dict か $%s でシステム辞書のパスを渡してください）", sudachi.DictEnv)
	}
	tk, err := sudachi.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("エラー: 辞書を開けません: %w", err)
	}
	return tk, tk.Close, nil
}

func newEmbedder(cfg cli.EmbedderConfig) (port.Embedder, error) {
	return embedding.NewHTTPEmbedder(embedding.Config{
		Endpoint: cfg.Endpoint,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
	})
}
