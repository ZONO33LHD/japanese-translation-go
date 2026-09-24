package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/interface/presenter"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
)

func genreChoices() []string {
	out := make([]string, len(model.Genres))
	for i, g := range model.Genres {
		out[i] = string(g)
	}
	return out
}

func runLint(_ context.Context, app *App, args []string) int {
	flags := app.newFlagSet("lint", "[--json] [--genre essay|tech|business] [--experimental] [--reading-load] [--baseline PREV.json] [--dict PATH] <file>")
	asJSON := flags.Bool("json", false, "機械可読な JSON で出力する")
	baselinePath := flags.String("baseline", "", "前回の --json 出力と比較し、resolved（解消）/ new（新規）/ persisting（継続）を判定する")
	genre := &choiceFlag{choices: genreChoices()}
	flags.Var(genre, "genre", "ジャンル別に校正した閾値プロファイルを適用する（essay/tech/business）。未指定時は共通の保守的閾値")
	experimental := flags.Bool("experimental", false, "まだ定量校正されていない、または無反応と判定された検出器も出力する")
	readingLoad := flags.Bool("reading-load", false, "読解負荷レーン（一文長・埋もれた列挙・連続漢字・二重否定・「の」連鎖）を併せて出力する")
	dict := flags.String("dict", "", "Sudachi システム辞書のパス（省略時は $SUDACHIN_DICT）")
	path, code, ok := app.parseSingleFile(flags, args)
	if !ok {
		return code
	}
	g, _ := model.ParseGenre(genre.value)

	text, err := app.Reader.Read(path)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}

	var baseline []lint.BaselineFinding
	useBaseline := false
	if *baselinePath != "" {
		baseline, useBaseline, code, ok = app.loadBaseline(*baselinePath)
		if !ok {
			return code
		}
	}

	tk, closeTk, err := app.OpenTokenizer(*dict)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	defer func() { _ = closeTk() }()

	// lint と読解負荷レーンで同じ解析結果を使い、形態素解析を 1 回で済ませる。
	doc, err := lint.NewService(tk).Prepare(text)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	findings, stats := lint.Lint(doc, lint.Options{Genre: g, Experimental: *experimental})
	report := presenter.LintReport{File: path, Findings: findings, Stats: stats}

	// 読解負荷レーンは findings にも stats にも混ぜず、baseline 比較にも渡さない。
	if *readingLoad {
		rl, rlStats := lint.ReadingLoad(doc, g)
		report.ReadingLoad = &presenter.ReadingLoadReport{Findings: rl, Stats: rlStats}
	}
	if useBaseline {
		marked, resolved, summary := lint.CompareBaseline(findings, baseline)
		report.Findings = marked
		report.Baseline = &presenter.BaselineReport{File: *baselinePath, Summary: summary, Resolved: resolved}
	}

	if *asJSON {
		if err := presenter.WriteLintJSON(app.Stdout, report); err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		return ExitOK
	}
	presenter.WriteLintHuman(app.Stdout, report)
	// lint であって CI ゲートではないので、検出件数に関わらず 0 で終える。
	return ExitOK
}

// loadBaseline は --baseline を読み込む。読めない・JSON として壊れている場合は入力エラー。
// 形が想定外なら警告を出して比較を諦め、通常の lint を続ける（useBaseline=false）。
// SourceReader を通すので UTF-8 検査と改行の正規化は対象文書と同じに効く。
func (app *App) loadBaseline(path string) (findings []lint.BaselineFinding, useBaseline bool, code int, ok bool) {
	reader := app.BaselineReader
	if reader == nil {
		reader = app.Reader
	}
	data, err := reader.Read(path)
	if err != nil {
		fmt.Fprintf(app.Stderr, "エラー: --baseline を読み込めません: %s\n", strings.TrimPrefix(err.Error(), "エラー: "))
		return nil, false, ExitInputError, false
	}
	findings, warnings, valid, err := presenter.DecodeBaseline([]byte(data))
	if err != nil {
		fmt.Fprintf(app.Stderr, "エラー: --baseline ファイルを読み込めません: %s (%v)\n", path, err)
		return nil, false, ExitInputError, false
	}
	for _, w := range warnings {
		fmt.Fprintf(app.Stderr, "警告: %s\n", w)
	}
	return findings, valid, ExitOK, true
}
