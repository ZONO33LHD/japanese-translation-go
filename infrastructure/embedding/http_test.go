package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type serverFunc func(w http.ResponseWriter, req embeddingRequest)

// newServer は受け取ったリクエストを検査しつつ fn に応答を任せるテストサーバー。
func newServer(t *testing.T, fn serverFunc) (*httptest.Server, *[]embeddingRequest) {
	t.Helper()
	var reqs []embeddingRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/embeddings" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		var req embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "" && auth != "Bearer secret" {
			http.Error(w, "bad auth "+auth, http.StatusUnauthorized)
			return
		}
		reqs = append(reqs, req)
		fn(w, req)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

// reversedResponder は index を逆順に並べて返し、クライアントが index で並べ直すことを確かめる。
func reversedResponder(w http.ResponseWriter, req embeddingRequest) {
	type item struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	}
	data := make([]item, 0, len(req.Input))
	for i := len(req.Input) - 1; i >= 0; i-- {
		var n float64
		fmt.Sscanf(req.Input[i], "s%g", &n)
		data = append(data, item{Index: i, Embedding: []float64{n, 1}})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func inputs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("s%d", i)
	}
	return out
}

func TestEmbedBatchesAndOrdersByIndex(t *testing.T) {
	srv, reqs := newServer(t, reversedResponder)
	e, err := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m", APIKey: "secret", BatchSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	vecs, err := e.Embed(context.Background(), inputs(7))
	if err != nil {
		t.Fatal(err)
	}
	if len(*reqs) != 3 {
		t.Errorf("requests = %d, want 3", len(*reqs))
	}
	for _, r := range *reqs {
		if r.Model != "m" {
			t.Errorf("model = %q", r.Model)
		}
	}
	for i, v := range vecs {
		if v[0] != float64(i) {
			t.Errorf("vecs[%d] = %v, want first component %d", i, v, i)
		}
	}
}

func TestEmbedRejectsCountMismatch(t *testing.T) {
	srv, _ := newServer(t, func(w http.ResponseWriter, _ embeddingRequest) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,2]}]}`))
	})
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m"})
	if _, err := e.Embed(context.Background(), inputs(2)); err == nil || !strings.Contains(err.Error(), "1 embeddings for 2 inputs") {
		t.Errorf("err = %v", err)
	}
}

func TestEmbedRejectsDimensionMismatchAcrossBatches(t *testing.T) {
	calls := 0
	srv, _ := newServer(t, func(w http.ResponseWriter, _ embeddingRequest) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,2]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,2,3]}]}`))
	})
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m", BatchSize: 1})
	if _, err := e.Embed(context.Background(), inputs(2)); err == nil || !strings.Contains(err.Error(), "dimension mismatch") {
		t.Errorf("err = %v", err)
	}
}

func TestEmbedReportsHTTPError(t *testing.T) {
	srv, _ := newServer(t, func(w http.ResponseWriter, _ embeddingRequest) {
		http.Error(w, "model not loaded", http.StatusServiceUnavailable)
	})
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m"})
	_, err := e.Embed(context.Background(), inputs(1))
	if err == nil || !strings.Contains(err.Error(), "status 503") || !strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("err = %v", err)
	}
}

func TestEmbedRejectsBadIndex(t *testing.T) {
	srv, _ := newServer(t, func(w http.ResponseWriter, _ embeddingRequest) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[1]}]}`))
	})
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m"})
	if _, err := e.Embed(context.Background(), inputs(2)); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err = %v", err)
	}
}

func TestEmbedHonorsContextCancel(t *testing.T) {
	srv, _ := newServer(t, reversedResponder)
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Embed(ctx, inputs(1)); err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestResolveURL(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080":                 "http://localhost:8080/v1/embeddings",
		"http://localhost:8080/":                "http://localhost:8080/v1/embeddings",
		"http://localhost:11434/v1":             "http://localhost:11434/v1/embeddings",
		"https://api.example.com/v1/embeddings": "https://api.example.com/v1/embeddings",
		"http://h/embeddings":                   "http://h/embeddings",
	}
	for in, want := range cases {
		got, err := resolveURL(in)
		if err != nil || got.String() != want {
			t.Errorf("resolveURL(%q) = %v, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "localhost:8080", "ftp://h", "/v1"} {
		if _, err := resolveURL(bad); err == nil {
			t.Errorf("resolveURL(%q) should fail", bad)
		}
	}
}

func TestNewHTTPEmbedderRequiresModel(t *testing.T) {
	if _, err := NewHTTPEmbedder(Config{Endpoint: "http://h"}); err == nil {
		t.Error("expected error for empty model")
	}
}

func TestEmbedRejectsOversizedResponse(t *testing.T) {
	old := maxResponseBody
	maxResponseBody = 64
	t.Cleanup(func() { maxResponseBody = old })
	srv, _ := newServer(t, func(w http.ResponseWriter, _ embeddingRequest) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[` + strings.Repeat("0.1,", 100) + `0.1]}]}`))
	})
	e, _ := NewHTTPEmbedder(Config{Endpoint: srv.URL, Model: "m"})
	_, err := e.Embed(context.Background(), inputs(1))
	if err == nil || !strings.Contains(err.Error(), "exceeds 64 bytes") {
		t.Errorf("err = %v", err)
	}
}

func TestErrorsRedactEndpointCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	u.User = url.UserPassword("user", "s3cret")
	e, err := NewHTTPEmbedder(Config{Endpoint: u.String(), Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Embed(context.Background(), []string{"a"})
	if err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error must not contain the password: %v", err)
	}
	if _, err := NewHTTPEmbedder(Config{Endpoint: "ftp://user:s3cret@h", Model: "m"}); err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("validation error must not contain the password: %v", err)
	}
}

// 接続に失敗したとき net/http のエラーはリクエスト URL を丸ごと含む。ユーザー名やクエリに
// 入れたトークンがそこから漏れないこと。
func TestTransportErrorDoesNotLeakURLSecrets(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	e, err := NewHTTPEmbedder(Config{Endpoint: "http://tok3nUser@" + addr + "/?api_key=tok3nQuery", Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if strings.Contains(err.Error(), "tok3n") {
		t.Errorf("secret leaked: %v", err)
	}
}
