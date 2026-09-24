package model

import "testing"

func TestParseGenre(t *testing.T) {
	cases := []struct {
		in     string
		want   Genre
		wantOK bool
	}{
		{"", GenreNone, true},
		{"essay", GenreEssay, true},
		{"tech", GenreTech, true},
		{"business", GenreBusiness, true},
		{"poem", GenreNone, false},
		{"Tech", GenreNone, false},
	}
	for _, c := range cases {
		got, ok := ParseGenre(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("ParseGenre(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
