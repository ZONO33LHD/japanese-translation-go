package pyfmt

import "testing"

func TestRepr(t *testing.T) {
	cases := map[float64]string{
		0:                   "0.0",
		3:                   "3.0",
		0.25:                "0.25",
		-0.24:               "-0.24",
		0.20689655172413793: "0.20689655172413793",
		1e-05:               "1e-05",
		0.0001:              "0.0001",
		1e16:                "1e+16",
		123456789012345.0:   "123456789012345.0",
		1.5e-10:             "1.5e-10",
	}
	for in, want := range cases {
		if got := Repr(in); got != want {
			t.Errorf("Repr(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPercentAndFixed(t *testing.T) {
	if got := Percent(0.36363636363636365, 1); got != "36.4%" {
		t.Errorf("Percent = %q", got)
	}
	if got := Percent(0.3, 0); got != "30%" {
		t.Errorf("Percent = %q", got)
	}
	if got := Fixed(-0.6234, 3); got != "-0.623" {
		t.Errorf("Fixed = %q", got)
	}
}

func TestRound(t *testing.T) {
	cases := []struct {
		in   float64
		n    int
		want float64
	}{
		{2.675, 2, 2.67},
		{0.125, 2, 0.12},
		{0.3333333, 3, 0.333},
		{12.345, 2, 12.35},
	}
	for _, c := range cases {
		if got := Round(c.in, c.n); got != c.want {
			t.Errorf("Round(%v, %d) = %v, want %v", c.in, c.n, got, c.want)
		}
	}
}

func TestIntList(t *testing.T) {
	if got := IntList([]int{2, 3, 3}); got != "[2, 3, 3]" {
		t.Errorf("IntList = %q", got)
	}
	if got := IntList(nil); got != "[]" {
		t.Errorf("IntList(nil) = %q", got)
	}
}
