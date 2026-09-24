package pystr

import (
	"testing"
)

func TestSlice(t *testing.T) {
	s := "あいうえお"
	cases := []struct {
		start, end int
		want       string
	}{
		{0, 2, "あい"},
		{3, 10, "えお"},
		{5, 8, ""},
		{-3, 1, "あ"},
		{2, 1, ""},
	}
	for _, c := range cases {
		if got := Slice(s, c.start, c.end); got != c.want {
			t.Errorf("Slice(%d,%d) = %q, want %q", c.start, c.end, got, c.want)
		}
	}
}

func TestFind(t *testing.T) {
	if got := Find("これはペンです", "ペン", 0); got != 3 {
		t.Errorf("Find = %d", got)
	}
	if got := Find("ペンとペン", "ペン", 1); got != 3 {
		t.Errorf("Find from = %d", got)
	}
	if got := Find("abc", "z", 0); got != -1 {
		t.Errorf("Find missing = %d", got)
	}
}

func TestStripHandlesFullWidthSpace(t *testing.T) {
	if got := Strip("　 本文\x1c "); got != "本文" {
		t.Errorf("Strip = %q", got)
	}
}

func TestRuneCursor(t *testing.T) {
	s := "aあbい"
	c := NewRuneCursor(s)
	for _, tc := range []struct{ b, want int }{{0, 0}, {1, 1}, {4, 2}, {8, 4}, {1, 1}} {
		if got := c.At(tc.b); got != tc.want {
			t.Errorf("At(%d) = %d, want %d", tc.b, got, tc.want)
		}
	}
}
