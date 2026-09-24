package cli

import (
	"context"
	"fmt"
)

const dictUsage = "install [--force] | path"

// runDict は辞書の取得（install）と場所の表示（path）を行う。
// 取得先は固定の Release、照合する SHA-256 も固定なので、利用者が URL や版を選ぶ余地は設けない。
func runDict(ctx context.Context, app *App, args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintf(app.Stderr, "usage: natural-japanese dict %s\n", dictUsage)
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	if app.Dictionary == nil {
		fmt.Fprintln(app.Stderr, "エラー: 辞書の保存先を決められません")
		return ExitInputError
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(app.Stdout, app.Dictionary.Path())
		if !app.Dictionary.Installed() {
			fmt.Fprintln(app.Stderr, "（まだ取得していません。natural-japanese dict install で取得できます）")
		}
		return ExitOK
	case "install":
		fs := app.newFlagSet("dict install", "[--force]")
		force := fs.Bool("force", false, "取得済みでも取得し直す")
		pos, code, ok := app.parseFlags(fs, args[1:])
		if !ok {
			return code
		}
		if len(pos) > 0 {
			fmt.Fprintf(app.Stderr, "余分な引数があります: %v\n", pos)
			return ExitUsage
		}
		if app.Dictionary.Installed() && !*force {
			fmt.Fprintf(app.Stdout, "取得済みです: %s\n", app.Dictionary.Path())
			return ExitOK
		}
		fmt.Fprintln(app.Stderr, "SudachiDict core を取得しています（約 72 MB。Apache License 2.0、zip 内の LEGAL を参照）…")
		path, err := app.Dictionary.Install(ctx)
		if err != nil {
			fmt.Fprintf(app.Stderr, "エラー: 辞書を取得できません: %v\n", err)
			return ExitInputError
		}
		fmt.Fprintf(app.Stdout, "取得しました: %s\n", path)
		return ExitOK
	default:
		fmt.Fprintf(app.Stderr, "不明なサブコマンドです: %s\nusage: natural-japanese dict %s\n", args[0], dictUsage)
		return ExitUsage
	}
}
