// Package filesystem はローカルファイルからの文書読み込みを提供する。
package filesystem

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

// MaxSourceBytes は検査対象ファイルの上限。信頼できない文書を渡されても
// メモリと CPU を使い切らないよう、仕事の文書として十分な大きさで打ち切る。
const MaxSourceBytes = 20 << 20

// maxSourceBytes はテストで小さくするための実際の上限値。
var maxSourceBytes int64 = MaxSourceBytes

// SourceReader は検査対象を UTF-8 テキストとして読み込む port.SourceReader の実装。
//
// 「文章の中身に関する判断」と「そもそも実行できない入力エラー」を区別するため、
// 入力エラーは利用者向けのメッセージを持つ error として返し、呼び出し側が exit 1 にする。
type SourceReader struct {
	// MaxBytes は読み込む上限。0 なら MaxSourceBytes。前回の lint --json 出力のように
	// 文書より大きくなりうる入力は、呼び出し側で広げる。
	MaxBytes int64
}

// BaselineMaxBytes は --baseline（前回の lint --json 出力）の上限。
// 出力は related_lines の分だけ入力より大きくなるので、文書の上限より広く取る。
const BaselineMaxBytes = 256 << 20

func (r SourceReader) limit() int64 {
	if r.MaxBytes > 0 {
		return r.MaxBytes
	}
	return maxSourceBytes
}

// Read は path を読み込む。
func (r SourceReader) Read(path string) (string, error) {
	maxBytes := r.limit()
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("エラー: ファイルが見つかりません: %s", path)
	}
	if err == nil && info.IsDir() {
		return "", fmt.Errorf("エラー: ディレクトリが指定されました（ファイルを指定してください）: %s", path)
	}
	if err == nil && info.Mode().IsRegular() && info.Size() > maxBytes {
		return "", tooLargeError(path, maxBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("エラー: ファイルを読み込めません: %s (%w)", path, err)
	}
	defer f.Close()
	// Stat のサイズを信用できないファイル（FIFO や読み込み中に伸びるファイル）にも上限を効かせる。
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("エラー: ファイルを読み込めません: %s (%w)", path, err)
	}
	if int64(len(b)) > maxBytes {
		return "", tooLargeError(path, maxBytes)
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("エラー: ファイルを読み込めません: %s (invalid UTF-8)", path)
	}
	return normalizeNewlines(string(b)), nil
}

func tooLargeError(path string, maxBytes int64) error {
	return fmt.Errorf("エラー: ファイルが大きすぎます（上限 %d MiB）: %s", maxBytes>>20, path)
}

// normalizeNewlines は CRLF / CR を LF に揃える。元実装（Python の read_text）の
// universal newlines と同じ扱いにして、行番号とオフセットの計算を一致させる。
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
