package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/internal/testenv"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/filesystem"
	"github.com/ZONO33LHD/japanese-translation-go/infrastructure/sudachi"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// calibrateFakeTokenizer は 1 文字 = 1 名詞の形態素を返す。CLI の配線確認にだけ使う。
type calibrateFakeTokenizer struct{}

func (calibrateFakeTokenizer) Tokenize(text string) ([]model.Morpheme, error) {
	var out []model.Morpheme
	for i, r := range []rune(text) {
		out = append(out, model.Morpheme{Surface: string(r), POS: [6]string{"名詞"}, DictionaryForm: string(r), Begin: i, End: i + 1})
	}
	return out, nil
}

func newCalibrateApp(open TokenizerOpener) (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errBuf bytes.Buffer
	return &App{
		OpenTokenizer:   open,
		NewCorpusLoader: func(dir string) port.CorpusLoader { return filesystem.CorpusLoader{Dir: dir} },
		Stdout:          &out,
		Stderr:          &errBuf,
	}, &out, &errBuf
}

func fakeOpener(string) (port.Tokenizer, func() error, error) {
	return calibrateFakeTokenizer{}, func() error { return nil }, nil
}

func writeCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"human/aozora/a.txt":  "これは人間の文章です。短い。\n\nもう一つの段落。",
		"ai/m/business-01.md": "重要なのは、結論として言えることです。このように整理できます。",
	}
	for p, c := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCalibrateReportWritesFiles(t *testing.T) {
	dir := writeCorpus(t)
	out := filepath.Join(t.TempDir(), "reports")
	app, stdout, stderr := newCalibrateApp(fakeOpener)
	if code := app.Run(context.Background(), []string{"calibrate", "report", "--corpus", dir, "--out", out}); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(out, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SampleCounts map[string]int `json:"sample_counts"`
		Matrix       map[string]map[string]struct {
			NDocs     int `json:"n_docs"`
			FiredDocs int `json:"fired_docs"`
		} `json:"matrix"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.SampleCounts["human_aozora"] != 1 || got.SampleCounts["ai"] != 1 || got.SampleCounts["ai_business"] != 1 {
		t.Errorf("sample_counts = %v", got.SampleCounts)
	}
	if c := got.Matrix["forbidden_phrase"]["ai"]; c.FiredDocs != 1 {
		t.Errorf("forbidden_phrase/ai = %+v", c)
	}
	if !strings.Contains(stdout.String(), "書き出し: "+filepath.Join(out, "report.md")) {
		t.Errorf("stdout missing output path: %s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "report.md")); err != nil {
		t.Error(err)
	}
}

func TestCalibrateSweepValidation(t *testing.T) {
	dir := writeCorpus(t)
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing detector", []string{"calibrate", "sweep", "--corpus", dir}, ExitUsage},
		{"unknown detector", []string{"calibrate", "sweep", "--detector", "nope", "--corpus", dir}, ExitInputError},
		{"unknown subcommand", []string{"calibrate", "bogus"}, ExitUsage},
		{"no subcommand", []string{"calibrate"}, ExitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app, _, _ := newCalibrateApp(fakeOpener)
			if code := app.Run(context.Background(), c.args); code != c.want {
				t.Errorf("exit %d, want %d", code, c.want)
			}
		})
	}
}

func TestCalibrateSweepAndLengthAnalysis(t *testing.T) {
	dir := writeCorpus(t)
	out := t.TempDir()
	app, _, stderr := newCalibrateApp(fakeOpener)
	if code := app.Run(context.Background(), []string{"calibrate", "sweep", "--detector", "antithesis_repetition", "--corpus", dir, "--out", out}); code != ExitOK {
		t.Fatalf("sweep exit %d: %s", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(out, "sweep_antithesis_repetition.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sweep struct {
		Default any   `json:"default"`
		Curve   []any `json:"curve"`
		NHuman  int   `json:"n_human"`
	}
	if err := json.Unmarshal(raw, &sweep); err != nil {
		t.Fatal(err)
	}
	if len(sweep.Curve) != 8 || sweep.NHuman != 1 || sweep.Default != float64(3) {
		t.Errorf("sweep = %+v", sweep)
	}
	if !bytes.Contains(raw, []byte(`"default": 3,`)) {
		t.Errorf("integer threshold should be written as an int: %s", raw[:120])
	}

	if code := app.Run(context.Background(), []string{"calibrate", "length-analysis", "--corpus", dir, "--out", out}); code != ExitOK {
		t.Fatalf("length-analysis exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(out, "length_analysis.md")); err != nil {
		t.Error(err)
	}
}

func TestCalibrateReportWithRealDictionary(t *testing.T) {
	path := testenv.RequireDict(t)
	open := func(string) (port.Tokenizer, func() error, error) {
		tk, err := sudachi.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return tk, tk.Close, nil
	}
	dir := writeCorpus(t)
	out := t.TempDir()
	app, _, stderr := newCalibrateApp(open)
	if code := app.Run(context.Background(), []string{"calibrate", "report", "--corpus", dir, "--out", out}); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	md, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "| forbidden_phrase | 0% / 0.00 | - | - | 100% / ") {
		t.Errorf("unexpected forbidden_phrase row:\n%s", md)
	}
}

func TestCalibrateEmptyCorpusIsInputError(t *testing.T) {
	app, _, errOut := newCalibrateApp(fakeOpener)
	if code := app.Run(context.Background(), []string{"calibrate", "report", "--corpus", t.TempDir()}); code != ExitInputError {
		t.Fatalf("exit = %d, want %d", code, ExitInputError)
	}
	if !strings.Contains(errOut.String(), "コーパスに文書がありません") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

// コーパスのファイル名は信頼できない。読み飛ばし警告に ESC をそのまま出さないこと。
func TestCalibrateSkippedFileNameIsEscaped(t *testing.T) {
	dir := writeCorpus(t)
	evil := filepath.Join(dir, "human", "aozora", "evil\x1b]2;PWNED\x07.txt")
	if err := os.WriteFile(evil, []byte("\xff\xfe"), 0o600); err != nil {
		t.Skip("filesystem rejects control characters in names:", err)
	}
	app, _, errOut := newCalibrateApp(fakeOpener)
	if code := app.Run(context.Background(), []string{"calibrate", "report", "--corpus", dir, "--out", t.TempDir()}); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	if strings.ContainsAny(errOut.String(), "\x1b\x07") {
		t.Fatalf("raw control characters reached stderr: %q", errOut)
	}
	if !strings.Contains(errOut.String(), `evil\x1b]2;PWNED\x07.txt`) {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestCalibrateReportFilePermissions(t *testing.T) {
	out := filepath.Join(t.TempDir(), "reports")
	app, _, errOut := newCalibrateApp(fakeOpener)
	if code := app.Run(context.Background(), []string{"calibrate", "report", "--corpus", writeCorpus(t), "--out", out}); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %q", code, errOut)
	}
	info, err := os.Stat(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Errorf("report.md mode = %v, must not be world-accessible", info.Mode().Perm())
	}
}
