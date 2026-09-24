package testenv

import (
	"runtime"
	"testing"
)

// fakeTB は testing.T と同じく Fatal / Skip で呼び出し側の goroutine を止め、その種類を記録する。
type fakeTB struct {
	testing.TB
	fatal, skipped bool
}

func (f *fakeTB) Helper()      {}
func (f *fakeTB) Fatal(...any) { f.fatal = true; runtime.Goexit() }
func (f *fakeTB) Skip(...any)  { f.skipped = true; runtime.Goexit() }

func requireDictIn(f *fakeTB) string {
	var got string
	done := make(chan struct{})
	go func() {
		defer close(done)
		got = RequireDict(f)
	}()
	<-done
	return got
}

func TestRequireDict(t *testing.T) {
	cases := []struct {
		name        string
		dict, ci    string
		want        string
		wantFatal   bool
		wantSkipped bool
	}{
		{"dict set", "/x/system_core.dic", "", "/x/system_core.dic", false, false},
		{"dict set in CI", "/x/system_core.dic", "true", "/x/system_core.dic", false, false},
		{"missing locally skips", "", "", "", false, true},
		{"missing in CI fails", "", "true", "", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(DictEnv, c.dict)
			t.Setenv("CI", c.ci)
			f := &fakeTB{}
			got := requireDictIn(f)
			if got != c.want || f.fatal != c.wantFatal || f.skipped != c.wantSkipped {
				t.Errorf("got=%q fatal=%v skipped=%v", got, f.fatal, f.skipped)
			}
		})
	}
}
