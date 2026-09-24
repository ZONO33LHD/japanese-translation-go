// Package pystr は元実装（Python の str）と同じ意味論の文字列操作を提供する。
// 文字数・オフセットはすべてルーン（コードポイント）単位で扱う。
package pystr

import (
	"strings"
	"unicode/utf8"
)

// WS は Python の正規表現 \s（str パターン）に相当する RE2 の文字クラス本体。
// Go の \s は ASCII のみなので、全角空白などを含めるためにこちらを使う。
const WS = `\t\n\v\f\r \x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

// IsSpace は Python の str.isspace() と同じ判定を 1 文字に対して行う。
func IsSpace(r rune) bool {
	switch {
	case r == ' ', r >= '\t' && r <= '\r', r >= 0x1c && r <= 0x1f:
		return true
	case r == 0x85, r == 0xa0, r == 0x1680, r >= 0x2000 && r <= 0x200a,
		r == 0x2028, r == 0x2029, r == 0x202f, r == 0x205f, r == 0x3000:
		return true
	}
	return false
}

// Strip は Python の str.strip() と同じく前後の空白を除く。
func Strip(s string) string { return strings.TrimFunc(s, IsSpace) }

// IsBlank は s.strip() == "" と同じ。
func IsBlank(s string) bool { return Strip(s) == "" }

// Len はルーン数（Python の len(str)）を返す。
func Len(s string) int { return utf8.RuneCountInString(s) }

// Slice は Python の s[start:end]（非負インデックス、範囲外は切り詰め）と同じ部分文字列を返す。
func Slice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end <= start {
		return ""
	}
	bStart, bEnd := -1, len(s)
	i := 0
	for b := range s {
		if i == start {
			bStart = b
		}
		if i == end {
			bEnd = b
			break
		}
		i++
	}
	if bStart < 0 {
		return ""
	}
	return s[bStart:bEnd]
}

// Head は s[:n] を返す。
func Head(s string, n int) string { return Slice(s, 0, n) }

// Find は Python の s.find(sub, from) と同じくルーン単位の位置を返す（見つからなければ -1）。
func Find(s, sub string, from int) int {
	offset := ByteIndex(s, from)
	if offset < 0 {
		if sub == "" && from == Len(s) {
			return from
		}
		return -1
	}
	idx := strings.Index(s[offset:], sub)
	if idx < 0 {
		return -1
	}
	return from + utf8.RuneCountInString(s[offset:offset+idx])
}

// ByteIndex はルーン位置 r に対応するバイト位置を返す（r が末尾ちょうどなら len(s)、範囲外は -1）。
func ByteIndex(s string, r int) int {
	if r < 0 {
		return -1
	}
	i := 0
	for b := range s {
		if i == r {
			return b
		}
		i++
	}
	if i == r {
		return len(s)
	}
	return -1
}

// RuneCursor は昇順に与えられるバイト位置をルーン位置へ変換する。
// マッチごとに先頭から数え直すと長い 1 行で O(n²) になるため、前回の位置から数え進める。
type RuneCursor struct {
	s          string
	byte, rune int
}

// NewRuneCursor は s 用のカーソルを作る。
func NewRuneCursor(s string) *RuneCursor { return &RuneCursor{s: s} }

// At はバイト位置 b のルーン位置を返す。b が前回より手前なら先頭から数え直す。
func (c *RuneCursor) At(b int) int {
	if b < c.byte {
		c.byte, c.rune = 0, 0
	}
	c.rune += utf8.RuneCountInString(c.s[c.byte:b])
	c.byte = b
	return c.rune
}

// Reverse はルーン列を逆順にした新しいスライスを返す。
func Reverse[T any](xs []T) []T {
	out := make([]T, len(xs))
	for i, x := range xs {
		out[len(xs)-1-i] = x
	}
	return out
}
