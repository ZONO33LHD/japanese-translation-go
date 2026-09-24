package model

// Line は 1-indexed の行番号付きの行。
type Line struct {
	No   int
	Text string
}

// Sentence は行番号・マスク済みの文・原文の文の組。
type Sentence struct {
	Line   int
	Masked string
	Raw    string
	// RawOffset は Masked の先頭が Raw の何文字目に当たるか。文頭のインラインコードは
	// マスクで空白になり strip で落ちるので、原文側は先頭がその分ずれる。
	RawOffset int
}

// Genre は閾値プロファイルの切り替えに使う文書ジャンル。空文字は未指定。
type Genre string

const (
	GenreNone     Genre = ""
	GenreEssay    Genre = "essay"
	GenreTech     Genre = "tech"
	GenreBusiness Genre = "business"
)

// Genres は CLI の選択肢として使う既知ジャンル（辞書順）。
var Genres = []Genre{GenreBusiness, GenreEssay, GenreTech}

// ParseGenre は文字列を Genre に変換する。
func ParseGenre(s string) (Genre, bool) {
	if s == "" {
		return GenreNone, true
	}
	for _, g := range Genres {
		if string(g) == s {
			return g, true
		}
	}
	return GenreNone, false
}
