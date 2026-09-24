// Package cli はコマンドライン引数を解釈してユースケースを呼び出すコントローラ層。
//
// 終了コードの規律は元実装と同じ: 文章の中身に関する判断は件数に関わらず 0、
// ファイルが読めない等の「そもそも実行できない」入力エラーは 1、引数の誤りは 2。
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/interface/presenter"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// 終了コード。
const (
	ExitOK         = 0
	ExitInputError = 1
	ExitUsage      = 2
)

// TokenizerOpener は辞書パス（空なら既定の解決規則）から Tokenizer を開く。
// 返り値の close は呼び出し側が必ず呼ぶ。
type TokenizerOpener func(dictPath string) (tk port.Tokenizer, closeFn func() error, err error)

// EmbedderConfig は semantic サブコマンドが埋め込みバックエンドに渡す設定。
type EmbedderConfig struct {
	Endpoint string
	Model    string
	APIKey   string
}

// EmbedderFactory は設定から Embedder を作る。
type EmbedderFactory func(cfg EmbedderConfig) (port.Embedder, error)

// App はコマンド群が共有する依存関係。コンポジションルート（cmd/）で組み立てる。
type App struct {
	Reader port.SourceReader
	// BaselineReader は --baseline 用。前回の出力は文書より大きくなりうるので上限を分ける。nil なら Reader。
	BaselineReader port.SourceReader
	OpenTokenizer  TokenizerOpener
	NewEmbedder    EmbedderFactory
	// NewCorpusLoader はコーパスのディレクトリから CorpusLoader を作る（calibrate 用）。
	NewCorpusLoader func(dir string) port.CorpusLoader
	// Dictionary は dict サブコマンドが扱う辞書の置き場所。
	Dictionary DictionaryStore
	Stdout     io.Writer
	Stderr     io.Writer
}

// DictionaryStore は既定の場所への辞書の取得を担う。
type DictionaryStore interface {
	Path() string
	Installed() bool
	Install(ctx context.Context) (string, error)
}

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, app *App, args []string) int
}

func (app *App) commands() []command {
	return []command{
		{"lint", "AI臭い日本語表現を決定的に検出する", runLint},
		{"outline", "見出し・段落先頭文・箇条書きのスケルトンと見出し統計を抽出する", runOutline},
		{"terms", "専門用語候補を初出順に抽出する", runTerms},
		{"semantic", "文埋め込みで話題の平板さを検出する（EXPERIMENTAL）", runSemantic},
		{"calibrate", "コーパスで検出器の閾値を校正する（開発者向け）", runCalibrate},
		{"dict", "形態素解析の辞書（SudachiDict core）を取得する・場所を表示する", runDict},
	}
}

// Run はサブコマンドを振り分けて終了コードを返す。
func (app *App) Run(ctx context.Context, args []string) int {
	// stderr には埋め込みサーバーのエラー本文やコーパスのファイル名など信頼できない文字列が混ざるので、
	// 出力先ごと端末制御文字を無害化する。
	app.Stderr = presenter.NewTerminalSafeWriter(app.Stderr)
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		app.usage()
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	for _, c := range app.commands() {
		if c.name == args[0] {
			return c.run(ctx, app, args[1:])
		}
	}
	fmt.Fprintf(app.Stderr, "不明なコマンドです: %s\n\n", args[0])
	app.usage()
	return ExitUsage
}

func (app *App) usage() {
	fmt.Fprintln(app.Stderr, "usage: natural-japanese <command> [options] <file>")
	fmt.Fprintln(app.Stderr)
	fmt.Fprintln(app.Stderr, "コマンド:")
	for _, c := range app.commands() {
		fmt.Fprintf(app.Stderr, "  %-10s %s\n", c.name, c.summary)
	}
}

// newFlagSet は usage を stderr に出す FlagSet を作る。
// flag パッケージ自身のエラー文（英語）と -name 形式の一覧は出さず、parseInterspersed が
// 日本語のエラーと --name 形式の一覧を出す。
func (app *App) newFlagSet(name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(app.Stderr, "usage: natural-japanese %s %s\n", name, usageLine)
		printFlagDefaults(app.Stderr, fs)
	}
	return fs
}

// printFlagDefaults は flag.PrintDefaults と同じ内容を、README と揃えた --name 形式で書き出す。
func printFlagDefaults(w io.Writer, fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		typeName, usage := flag.UnquoteUsage(f)
		line := "  --" + f.Name
		if typeName != "" {
			line += " " + typeName
		}
		line += "\n    \t" + strings.ReplaceAll(usage, "\n", "\n    \t")
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			line += fmt.Sprintf("（既定: %s）", f.DefValue)
		}
		fmt.Fprintln(w, line)
	})
}

var (
	flagUndefinedRE    = regexp.MustCompile(`^flag provided but not defined: -+(.+)$`)
	flagNeedsArgRE     = regexp.MustCompile(`^flag needs an argument: -+(.+)$`)
	flagInvalidValueRE = regexp.MustCompile(`^invalid (?:boolean )?value "(.*)" for flag -+([^:]+): (.*)$`)
)

// flagErrorText は flag パッケージの英語のエラー文を利用者向けの日本語にする。
func flagErrorText(err error) string {
	msg := err.Error()
	if m := flagUndefinedRE.FindStringSubmatch(msg); m != nil {
		return "--" + m[1] + " というオプションはありません"
	}
	if m := flagNeedsArgRE.FindStringSubmatch(msg); m != nil {
		return "--" + m[1] + " には値が必要です"
	}
	if m := flagInvalidValueRE.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("--%s の値 %q が正しくありません（%s）", m[2], m[1], m[3])
	}
	return msg
}

// parseInterspersed はフラグと位置引数が混在していても解釈する（argparse と同じ使い勝手）。
// 例: `lint draft.md --json` と `lint --json draft.md` を同じに扱う。
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	// 解析中は flag パッケージに usage を出させない。-h とエラーは parseFlags が扱う。
	usage := fs.Usage
	fs.Usage = func() {}
	defer func() { fs.Usage = usage }()
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			return positional, nil
		}
		if rest[0] == "--" {
			return append(positional, rest[1:]...), nil
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
}

// parseFlags はフラグを解釈する。-h なら usage を出して exit 0、誤りなら日本語のエラーと usage を出して exit 2。
func (app *App) parseFlags(fs *flag.FlagSet, args []string) ([]string, int, bool) {
	pos, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fs.Usage()
		return nil, ExitOK, false
	}
	if err != nil {
		fmt.Fprintf(app.Stderr, "エラー: 引数が正しくありません: %s\n", flagErrorText(err))
		fs.Usage()
		return nil, ExitUsage, false
	}
	return pos, ExitOK, true
}

// parseSingleFile は位置引数がちょうど 1 つ（対象ファイル）であることを検査する。
func (app *App) parseSingleFile(fs *flag.FlagSet, args []string) (string, int, bool) {
	pos, code, ok := app.parseFlags(fs, args)
	if !ok {
		return "", code, false
	}
	if len(pos) != 1 {
		fmt.Fprintf(app.Stderr, "対象ファイルを 1 つだけ指定してください（%d 個指定されています）\n", len(pos))
		fs.Usage()
		return "", ExitUsage, false
	}
	return pos[0], ExitOK, true
}

// choiceFlag は選択肢を検査する文字列フラグ（argparse の choices 相当）。
type choiceFlag struct {
	value   string
	choices []string
}

func (c *choiceFlag) String() string { return c.value }

func (c *choiceFlag) Set(s string) error {
	for _, ch := range c.choices {
		if ch == s {
			c.value = s
			return nil
		}
	}
	return fmt.Errorf("選択肢は %s です", strings.Join(c.choices, " / "))
}
