package presenter

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// sanitizeTerminal は人間向け出力に書く前に、端末を操作できる文字を可視表記に置き換える。
// excerpt などは検査対象の文書から切り出した信頼できない文字列で、ESC を含むと
// OSC 52（クリップボード書き換え）やカーソル移動による表示の偽装を端末で実行されてしまうため。
// 改行とタブは用語の近傍など複数行にまたがる正当な表示に要るので残す。
// 双方向制御文字と行・段落区切りは、表示順の入れ替えや行の偽装に使えるので置き換える。
// JSON 出力は encoding/json が制御文字をエスケープするので対象にしない。
func sanitizeTerminal(s string) string {
	if !strings.ContainsFunc(s, isUnsafeRune) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		writeSafeRune(&b, r)
	}
	return b.String()
}

func writeSafeRune(b *strings.Builder, r rune) {
	switch {
	case isUnsafeControl(r):
		fmt.Fprintf(b, `\x%02x`, r)
	case isDisplaySpoofing(r):
		fmt.Fprintf(b, `\u{%04X}`, r)
	default:
		b.WriteRune(r)
	}
}

func isUnsafeRune(r rune) bool { return isUnsafeControl(r) || isDisplaySpoofing(r) }

// isUnsafeControl は C0 制御文字（改行・タブを除く）、DEL、C1 制御文字を判定する。
func isUnsafeControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// isDisplaySpoofing は双方向テキストの制御文字と Unicode の行・段落区切りを判定する。
func isDisplaySpoofing(r rune) bool {
	switch {
	case r == 0x200e || r == 0x200f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0x2028 || r == 0x2029:
		return true
	}
	return false
}

// terminalSafeWriter は書き込まれる内容に sanitizeTerminal と同じ置き換えをかける。
// 埋め込みサーバーのエラー本文やコーパスのファイル名のように、stderr のメッセージにも
// 信頼できない文字列が混ざるため、メッセージごとではなく出力先でまとめて守る。
type terminalSafeWriter struct {
	w       io.Writer
	pending []byte // 前回の Write の末尾で切れた UTF-8 の断片
}

// NewTerminalSafeWriter は w への書き込みから端末制御に使える文字を可視表記に置き換える Writer を返す。
func NewTerminalSafeWriter(w io.Writer) io.Writer {
	if _, ok := w.(*terminalSafeWriter); ok {
		return w
	}
	return &terminalSafeWriter{w: w}
}

func (t *terminalSafeWriter) Write(p []byte) (int, error) {
	data := p
	if len(t.pending) > 0 {
		data = append(t.pending, p...)
		t.pending = nil
	}
	// 末尾が不完全な UTF-8 なら次の Write まで持ち越す（C1 制御文字は 2 バイトなので分割されうる）。
	cut := len(data)
	for i := 1; i <= utf8.UTFMax && i <= len(data); i++ {
		c := data[len(data)-i]
		if c < 0x80 {
			break
		}
		if utf8.RuneStart(c) {
			if !utf8.FullRune(data[len(data)-i:]) {
				cut = len(data) - i
			}
			break
		}
	}
	if cut < len(data) {
		t.pending = append([]byte(nil), data[cut:]...)
	}
	if _, err := io.WriteString(t.w, sanitizeTerminal(string(data[:cut]))); err != nil {
		return 0, err
	}
	return len(p), nil
}
