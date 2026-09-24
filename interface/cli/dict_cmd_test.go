package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDict struct {
	installed bool
	calls     int
	err       error
}

func (f *fakeDict) Path() string    { return "/cache/system_core.dic" }
func (f *fakeDict) Installed() bool { return f.installed }
func (f *fakeDict) Install(context.Context) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	f.installed = true
	return f.Path(), nil
}

func TestDictCommand(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		dict      *fakeDict
		wantCode  int
		wantCalls int
		wantOut   string
	}{
		{"install", []string{"dict", "install"}, &fakeDict{}, ExitOK, 1, "取得しました"},
		{"already installed", []string{"dict", "install"}, &fakeDict{installed: true}, ExitOK, 0, "取得済み"},
		{"force", []string{"dict", "install", "--force"}, &fakeDict{installed: true}, ExitOK, 1, "取得しました"},
		{"failure", []string{"dict", "install"}, &fakeDict{err: errors.New("checksum mismatch")}, ExitInputError, 1, ""},
		{"path", []string{"dict", "path"}, &fakeDict{}, ExitOK, 0, "/cache/system_core.dic"},
		{"no subcommand", []string{"dict"}, &fakeDict{}, ExitUsage, 0, ""},
		{"unknown", []string{"dict", "remove"}, &fakeDict{}, ExitUsage, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			app := &App{Dictionary: c.dict, Stdout: &out, Stderr: &errOut}
			if code := app.Run(context.Background(), c.args); code != c.wantCode {
				t.Errorf("exit = %d, want %d (stderr %q)", code, c.wantCode, errOut.String())
			}
			if c.dict.calls != c.wantCalls {
				t.Errorf("Install calls = %d, want %d", c.dict.calls, c.wantCalls)
			}
			if !strings.Contains(out.String(), c.wantOut) {
				t.Errorf("stdout = %q, want %q", out.String(), c.wantOut)
			}
		})
	}
}
