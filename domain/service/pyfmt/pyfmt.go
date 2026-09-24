// Package pyfmt は元実装（Python）の数値表記と一致する書式化を提供する。
// 検出結果の detail 文字列や JSON の数値表記を元実装と揃え、出力を比較可能に保つ。
package pyfmt

import (
	"math"
	"strconv"
	"strings"
)

// Repr は Python の repr(float) / str(float) と同じ表記を返す（例: 3.0, 0.25, 1e-05）。
func Repr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	sci := strconv.FormatFloat(f, 'e', -1, 64) // 例: -1.2345e-05
	neg := strings.HasPrefix(sci, "-")
	sci = strings.TrimPrefix(sci, "-")
	mant, expStr, _ := strings.Cut(sci, "e")
	exp, _ := strconv.Atoi(expStr)
	digits := strings.Replace(mant, ".", "", 1)

	var out string
	if exp >= -4 && exp < 16 {
		out = fixedFromDigits(digits, exp)
	} else {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		sign := "+"
		if exp < 0 {
			sign = "-"
			exp = -exp
		}
		out = m + "e" + sign + leftPad2(exp)
	}
	if neg {
		return "-" + out
	}
	return out
}

func fixedFromDigits(digits string, exp int) string {
	pointPos := exp + 1
	switch {
	case pointPos <= 0:
		return "0." + strings.Repeat("0", -pointPos) + digits
	case pointPos >= len(digits):
		return digits + strings.Repeat("0", pointPos-len(digits)) + ".0"
	default:
		return digits[:pointPos] + "." + digits[pointPos:]
	}
}

func leftPad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// Fixed は Python の f"{x:.{n}f}" と同じ表記を返す。
func Fixed(f float64, n int) string { return strconv.FormatFloat(f, 'f', n, 64) }

// Percent は Python の f"{x:.{n}%}" と同じ表記を返す。
func Percent(f float64, n int) string { return strconv.FormatFloat(f*100, 'f', n, 64) + "%" }

// Round は Python の round(x, n) と同じ値を返す（10 進で正しく丸めた最近接値）。
func Round(f float64, n int) float64 {
	v, err := strconv.ParseFloat(strconv.FormatFloat(f, 'f', n, 64), 64)
	if err != nil {
		return f
	}
	return v
}

// IntList は Python の str(list[int]) と同じ表記（例: [3, 3, 2]）を返す。
func IntList(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// Float は JSON に Python の json.dumps と同じ表記（例: 0.0, 1e-05）で書き出される float64。
type Float float64

// MarshalJSON は float repr で数値を書き出す。NaN と ±Inf は JSON として不正で、
// --baseline で読み戻せなくなるため null にする。
func (f Float) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return []byte("null"), nil
	}
	return []byte(Repr(v)), nil
}
