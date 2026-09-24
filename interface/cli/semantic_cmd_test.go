package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

type semanticMemReader map[string]string

func (m semanticMemReader) Read(path string) (string, error) {
	s, ok := m[path]
	if !ok {
		return "", fmt.Errorf("エラー: ファイルが見つかりません: %s", path)
	}
	return s, nil
}

// orthoEmbedder は全文を互いに直交するベクトルにする（平板性・反復 max が発火する）。
type orthoEmbedder struct {
	model string
	calls int
	err   error
}

func (o *orthoEmbedder) Embed(_ context.Context, ss []string) ([][]float64, error) {
	o.calls++
	if o.err != nil {
		return nil, o.err
	}
	out := make([][]float64, len(ss))
	for i := range ss {
		out[i] = make([]float64, len(ss))
		out[i][i] = 1
	}
	return out, nil
}

func (o *orthoEmbedder) ModelName() string { return o.model }

func semanticDoc(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "文%dです。", i)
	}
	return b.String()
}

func newSemanticApp(files semanticMemReader, emb *orthoEmbedder, gotCfg *EmbedderConfig) (*App, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &App{
		Reader: files,
		NewEmbedder: func(cfg EmbedderConfig) (port.Embedder, error) {
			*gotCfg = cfg
			emb.model = cfg.Model
			return emb, nil
		},
		Stdout: &out,
		Stderr: &errOut,
	}, &out, &errOut
}

func TestSemanticCommandJSON(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, out, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	code := app.Run(context.Background(), []string{"semantic", "doc.md", "--json", "--genre", "tech", "--endpoint", "http://e:1", "--api-key", "k"})
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if cfg.Endpoint != "http://e:1" || cfg.APIKey != "k" || cfg.Model != "cl-nagoya/ruri-v3-310m" {
		t.Errorf("embedder config = %+v", cfg)
	}
	if !strings.Contains(errOut.String(), "[semantic] 埋め込みAPIに問い合わせ中: http://e:1") {
		t.Errorf("missing notice: %q", errOut)
	}
	var got struct {
		File  string `json:"file"`
		Stats struct {
			Genre             *string            `json:"genre"`
			NSentences        int                `json:"n_sentences"`
			FlatnessThreshold float64            `json:"flatness_threshold"`
			Metrics           map[string]float64 `json:"metrics"`
			Skipped           bool               `json:"skipped"`
			SkipReason        *string            `json:"skip_reason"`
		} `json:"stats"`
		Findings []struct {
			Category string `json:"category"`
			Line     int    `json:"line"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out)
	}
	if got.File != "doc.md" || got.Stats.Genre == nil || *got.Stats.Genre != "tech" || got.Stats.NSentences != 10 ||
		got.Stats.Skipped || got.Stats.SkipReason != nil || got.Stats.FlatnessThreshold != 0.14261949062347412 {
		t.Errorf("stats = %+v", got.Stats)
	}
	if len(got.Findings) != 2 || got.Findings[0].Category != "semantic_topic_flatness" {
		t.Errorf("findings = %+v", got.Findings)
	}
	wantOrder := []string{`"genre"`, `"n_sentences"`, `"model"`, `"flatness_threshold"`, `"metrics"`, `"skipped"`, `"skip_reason"`}
	last := -1
	for _, k := range wantOrder {
		idx := strings.Index(out.String(), k)
		if idx <= last {
			t.Errorf("key %s out of order", k)
		}
		last = idx
	}
}

func TestSemanticCommandSkipsShortDocumentWithoutBackend(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, out, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(3)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md"}); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if emb.calls != 0 || errOut.Len() != 0 {
		t.Errorf("backend must not be contacted: calls=%d stderr=%q", emb.calls, errOut)
	}
	want := "=== semantic (EXPERIMENTAL): doc.md ===\n文数: 3  モデル: cl-nagoya/ruri-v3-310m  genre: (未指定)\nスキップ: 文数が3文と少なく"
	if !strings.HasPrefix(out.String(), want) {
		t.Errorf("output = %q", out)
	}
}

func TestSemanticCommandBackendFailureExitsOne(t *testing.T) {
	emb := &orthoEmbedder{err: errors.New("connection refused")}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md"}); code != ExitInputError {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errOut.String(), "エラー: 意味モデルの読み込みまたは推論に失敗しました: ") ||
		!strings.Contains(errOut.String(), "connection refused") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSemanticCommandRejectsUnknownGenre(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, _, _ := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "--genre", "novel", "doc.md"}); code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
}

func TestSemanticCommandMissingFile(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "none.md"}); code != ExitInputError {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(errOut.String(), "ファイルが見つかりません") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestSemanticHelpDoesNotLeakAPIKey(t *testing.T) {
	t.Setenv(EmbeddingAPIKeyEnv, "sk-SECRET-123")
	t.Setenv(EmbeddingEndpointEnv, "https://internal.example/embed")
	for _, args := range [][]string{{"semantic", "-h"}, {"semantic", "--no-such-flag"}} {
		emb := &orthoEmbedder{}
		var cfg EmbedderConfig
		app, out, errOut := newSemanticApp(semanticMemReader{}, emb, &cfg)
		app.Run(context.Background(), args)
		all := out.String() + errOut.String()
		if strings.Contains(all, "sk-SECRET-123") || strings.Contains(all, "internal.example") {
			t.Errorf("%v leaked env values into usage:\n%s", args, all)
		}
	}
}

func TestSemanticFallsBackToEnvAfterParsing(t *testing.T) {
	t.Setenv(EmbeddingAPIKeyEnv, "sk-env")
	t.Setenv(EmbeddingEndpointEnv, "https://emb.example")
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md"}); code != ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if cfg.APIKey != "sk-env" || cfg.Endpoint != "https://emb.example" {
		t.Errorf("embedder config = %+v", cfg)
	}
	if strings.Contains(errOut.String(), "警告") {
		t.Errorf("https endpoint must not trigger the cleartext warning: %q", errOut)
	}
}

func TestSemanticWarnsOnCleartextRemoteEndpoint(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md", "--endpoint", "http://emb.example:8080"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.Count(errOut.String(), "暗号化されていない") != 1 {
		t.Errorf("want exactly one cleartext warning: %q", errOut)
	}
}

func TestIsCleartextRemote(t *testing.T) {
	cases := map[string]bool{
		"http://localhost:8080":       false,
		"http://127.0.0.1:8080":       false,
		"http://[::1]:8080":           false,
		"http://192.168.1.10/v1":      false,
		"http://10.0.0.2":             false,
		"http://api.localhost":        false,
		"https://emb.example":         false,
		"http://emb.example":          true,
		"http://8.8.8.8/v1":           true,
		"HTTP://Emb.Example/v1/embed": true,
		"http://169.254.169.254/":     true,
		"http://[fe80::1]/":           true,
		"http://[::ffff:10.0.0.1]/":   false,
		"http://[::ffff:8.8.8.8]/":    true,
		"http://0.0.0.0:8080":         false,
		"http://localhost./":          false,
		"http://localhost.evil.com/":  true,
		"http://localhost@evil.com/":  true,
	}
	for in, want := range cases {
		if got := isCleartextRemote(in); got != want {
			t.Errorf("isCleartextRemote(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSemanticMalformedEndpointIsUsageError(t *testing.T) {
	for _, ep := range []string{"localhost:8080", "ftp://h", "http://", "://x"} {
		emb := &orthoEmbedder{}
		var cfg EmbedderConfig
		app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
		if code := app.Run(context.Background(), []string{"semantic", "doc.md", "--endpoint", ep}); code != ExitUsage {
			t.Errorf("%q: exit = %d, want %d", ep, code, ExitUsage)
		}
		if !strings.Contains(errOut.String(), "--endpoint は") || emb.calls != 0 {
			t.Errorf("%q: stderr = %q, calls = %d", ep, errOut, emb.calls)
		}
	}
}

func TestSemanticRedactsEndpointCredentials(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md", "--endpoint", "http://user:s3cret@203.0.113.5:9/"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(errOut.String(), "s3cret") {
		t.Errorf("password leaked to stderr: %q", errOut)
	}
	if !strings.Contains(errOut.String(), "http://xxxxx@203.0.113.5:9/") {
		t.Errorf("notice should show the redacted endpoint: %q", errOut)
	}
}

// ユーザー名だけのトークン、クエリの鍵、scheme のない値も表示しない。
func TestSemanticRedactsTokensOutsidePassword(t *testing.T) {
	cases := []struct {
		endpoint string
		wantCode int
	}{
		{"http://tok3n@203.0.113.5:9/", ExitOK},
		{"https://emb.example/v1?key=tok3n", ExitOK},
		{"user:tok3n@host:8080", ExitUsage},
	}
	for _, c := range cases {
		emb := &orthoEmbedder{}
		var cfg EmbedderConfig
		app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
		if code := app.Run(context.Background(), []string{"semantic", "doc.md", "--endpoint", c.endpoint}); code != c.wantCode {
			t.Errorf("%s: exit = %d, want %d", c.endpoint, code, c.wantCode)
		}
		if strings.Contains(errOut.String(), "tok3n") {
			t.Errorf("%s: token leaked to stderr: %q", c.endpoint, errOut)
		}
	}
}

// 埋め込みサーバーのエラー本文は信頼できない。ESC や BEL をそのまま端末に出さないこと。
func TestSemanticBackendErrorBodyIsEscaped(t *testing.T) {
	emb := &orthoEmbedder{err: errors.New("status 400: \x1b]2;PWNED\x07 \x1b[31mFAKE")}
	var cfg EmbedderConfig
	app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
	if code := app.Run(context.Background(), []string{"semantic", "doc.md"}); code != ExitInputError {
		t.Fatalf("exit = %d", code)
	}
	if strings.ContainsAny(errOut.String(), "\x1b\x07") {
		t.Fatalf("raw control characters reached stderr: %q", errOut)
	}
	if !strings.Contains(errOut.String(), `\x1b]2;PWNED\x07`) {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestFlagErrorsAreJapanese(t *testing.T) {
	emb := &orthoEmbedder{}
	var cfg EmbedderConfig
	cases := map[string][]string{
		"--nope というオプションはありません":         {"semantic", "--nope", "doc.md"},
		"--genre の値 \"poem\" が正しくありません": {"semantic", "--genre", "poem", "doc.md"},
		"--model には値が必要です":              {"semantic", "doc.md", "--model"},
	}
	for want, args := range cases {
		app, _, errOut := newSemanticApp(semanticMemReader{"doc.md": semanticDoc(10)}, emb, &cfg)
		if code := app.Run(context.Background(), args); code != ExitUsage {
			t.Errorf("%v: exit = %d", args, code)
		}
		s := errOut.String()
		if !strings.Contains(s, "エラー: 引数が正しくありません: "+want) || !strings.Contains(s, "  --json") || strings.Contains(s, "flag provided") {
			t.Errorf("%v: stderr = %q", args, s)
		}
	}
}
