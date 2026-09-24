package cli

import (
	"context"
	"fmt"

	"github.com/ZONO33LHD/japanese-translation-go/interface/presenter"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/terms"
)

func runTerms(_ context.Context, app *App, args []string) int {
	fs := app.newFlagSet("terms", "[--json] [--dict PATH] <file>")
	asJSON := fs.Bool("json", false, "機械可読な JSON で出力する")
	dict := fs.String("dict", "", "Sudachi システム辞書のパス（省略時は $SUDACHIN_DICT）")
	path, code, ok := app.parseSingleFile(fs, args)
	if !ok {
		return code
	}

	text, err := app.Reader.Read(path)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	tk, closeTk, err := app.OpenTokenizer(*dict)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	defer func() { _ = closeTk() }()

	ts, err := terms.BuildInventory(text, tk)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	if *asJSON {
		if err := presenter.WriteJSON(app.Stdout, presenter.TermsJSON(ts)); err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		return ExitOK
	}
	presenter.WriteTermsHuman(app.Stdout, path, ts)
	return ExitOK
}
