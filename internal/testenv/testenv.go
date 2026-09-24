// Package testenv はテストの実行環境に関する共通の判定を提供する。
package testenv

import (
	"os"
	"testing"
)

// DictEnv は Sudachi システム辞書のパスを指定する環境変数。
const DictEnv = "SUDACHIN_DICT"

// RequireDict は辞書のパスを返す。辞書が無い手元環境ではスキップするが、CI では
// 辞書を使う回帰テストが黙って消えないよう失敗させる。
func RequireDict(t testing.TB) string {
	t.Helper()
	path := os.Getenv(DictEnv)
	if path != "" {
		return path
	}
	if os.Getenv("CI") != "" {
		t.Fatal("$" + DictEnv + " must be set in CI so dictionary-backed tests are not skipped")
	}
	t.Skip("$" + DictEnv + " is not set")
	return ""
}
