package model

import (
	"unicode"
)

// Morpheme は形態素解析器から独立した形態素の表現。
// Begin/End は解析対象文字列上のルーン（コードポイント）オフセットで、
// Python の str スライスと同じ単位で扱えるようにしている。
type Morpheme struct {
	Surface        string
	POS            [6]string
	DictionaryForm string
	NormalizedForm string
	ReadingForm    string
	Begin, End     int
}

// POS1 は品詞大分類（part_of_speech()[0]）。
func (m Morpheme) POS1() string { return m.POS[0] }

// POS2 は品詞中分類（part_of_speech()[1]）。
func (m Morpheme) POS2() string { return m.POS[1] }

// TokenizedSentence は 1 文とその形態素列。
type TokenizedSentence struct {
	Line int
	// Text はマスク済みテキスト（形態素解析・パターンマッチ用）。
	Text      string
	Morphemes []Morpheme
	// RawText は原文。excerpt 表示は必ずこちらから切り出す。
	RawText string
	// RawOffset は Text（と形態素のオフセット）の 0 文字目が RawText の何文字目に当たるか。
	RawOffset int
}

// RawSlice は形態素オフセット [begin, end) に対応する原文を返す。
// 同じ文から何度も切り出すときは Excerpter を使う（毎回ルーン列を作り直さないため）。
func (ts TokenizedSentence) RawSlice(begin, end int) string { return ts.Excerpter()(begin, end) }

// Excerpter は形態素オフセット [begin, end) から原文を切り出す関数を返す。
//
// 開始位置がマスクで空白になった区間（インラインコードやリンク URL）の途中に来ると
// 「y`を使う」のような断片になるので、その区間を読み飛ばしてから切り出す。
func (ts TokenizedSentence) Excerpter() func(begin, end int) string {
	raw := []rune(ts.RawText)
	maskedRun := maskedRuns([]rune(ts.Text), raw, ts.RawOffset)
	masked := func(p int) bool { return p < len(maskedRun) && maskedRun[p] }
	return func(begin, end int) string {
		begin = max(begin, 0)
		for begin < end && masked(begin) {
			begin++
		}
		b, e := min(begin+ts.RawOffset, len(raw)), min(end+ts.RawOffset, len(raw))
		if b >= e {
			return ""
		}
		return string(raw[b:e])
	}
}

// maskedRuns は Text 上の各位置が「マスクで空白化された区間」に属するかを返す。
// 1 文字ずつ比べると、コード中の空白（`x y z`）で区間が途切れて判定を誤るため、
// Text の空白の連なりを単位にし、原文側の同じ範囲に空白以外が 1 つでもあれば区間全体をマスクとみなす。
func maskedRuns(text, raw []rune, offset int) []bool {
	out := make([]bool, len(text))
	for a := 0; a < len(text); {
		if !unicode.IsSpace(text[a]) {
			a++
			continue
		}
		b := a
		for b < len(text) && unicode.IsSpace(text[b]) {
			b++
		}
		differs := false
		for p := a; p < b && p+offset < len(raw); p++ {
			if !unicode.IsSpace(raw[p+offset]) {
				differs = true
				break
			}
		}
		for p := a; p < b; p++ {
			out[p] = differs
		}
		a = b
	}
	return out
}
