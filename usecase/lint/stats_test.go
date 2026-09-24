package lint

import "testing"

// Python の statistics.pstdev は正しく丸めた平方根を返す。float64 で分散を割ってから
// math.Sqrt すると 11.841546445554409 になり最終桁がずれる。
func TestPstdevIntsIsCorrectlyRounded(t *testing.T) {
	cases := []struct {
		xs   []int
		want float64
	}{
		{[]int{31, 16, 2}, 11.841546445554407},
		{[]int{2, 4, 4, 4, 5, 5, 7, 9}, 2},
		{[]int{5, 5, 5}, 0},
	}
	for _, c := range cases {
		if got := pstdevInts(c.xs); got != c.want {
			t.Errorf("pstdevInts(%v) = %v, want %v", c.xs, got, c.want)
		}
	}
}
