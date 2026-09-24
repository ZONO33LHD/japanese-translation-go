// Package embedding は文埋め込みバックエンドとの通信を提供する。
//
// OpenAI 互換の POST /v1/embeddings を話すため、text-embeddings-inference・Ollama・
// llama.cpp server・vLLM などで cl-nagoya/ruri-v3-310m を配信すれば Python なしで動く。
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBatchSize は 1 リクエストあたりの文数。長文書でもリクエストサイズ上限に当たらないよう分割する。
	DefaultBatchSize = 64
	// DefaultTimeout は 1 リクエストあたりのタイムアウト。CPU 推論のサーバーでも収まる長さにしている。
	DefaultTimeout = 2 * time.Minute
	maxErrorBody   = 512
)

// maxResponseBody は成功応答の上限。64 文 × 数千次元の float でも十分収まる大きさにしている（テストで差し替えるため var）。
var maxResponseBody int64 = 64 << 20

// Config は埋め込み API の接続設定。
type Config struct {
	// Endpoint はサーバーのベース URL（例: http://localhost:8080）。/embeddings で終わる場合はそのまま使う。
	Endpoint string
	Model    string
	// APIKey は空なら Authorization ヘッダーを送らない。
	APIKey    string
	BatchSize int
	Timeout   time.Duration
	// HTTPClient はテスト等で差し替える場合に指定する。nil なら Timeout 付きの既定クライアント。
	HTTPClient *http.Client
}

// HTTPEmbedder は HTTP の埋め込み API を呼ぶ port.Embedder の実装。
type HTTPEmbedder struct {
	url string
	// displayURL はエラーメッセージ用に userinfo とクエリを伏せた URL。
	displayURL string
	model      string
	apiKey     string
	batchSize  int
	client     *http.Client
}

// NewHTTPEmbedder は設定を検証して HTTPEmbedder を作る。
func NewHTTPEmbedder(cfg Config) (*HTTPEmbedder, error) {
	if cfg.Model == "" {
		return nil, errors.New("embedding model name is empty")
	}
	u, err := resolveURL(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = DefaultBatchSize
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &HTTPEmbedder{url: u.String(), displayURL: redactURL(u), model: cfg.Model, apiKey: cfg.APIKey, batchSize: batch, client: client}, nil
}

func resolveURL(endpoint string) (*url.URL, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, errors.New("embedding endpoint is empty")
	}
	// URL に認証情報が含まれうるので、エラーには URL そのものを入れない。
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.New("embedding endpoint is not a valid URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("embedding endpoint must be an absolute http(s) URL")
	}
	switch {
	case strings.HasSuffix(u.Path, "/embeddings"):
	case strings.HasSuffix(u.Path, "/v1"):
		u.Path += "/embeddings"
	default:
		u.Path += "/v1/embeddings"
	}
	return u, nil
}

// ModelName はモデル名を返す。
func (e *HTTPEmbedder) ModelName() string { return e.model }

// URL は実際に呼び出すエンドポイントを返す。
func (e *HTTPEmbedder) URL() string { return e.url }

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

// Embed は sentences を入力順の埋め込みベクトル列に変換する。
func (e *HTTPEmbedder) Embed(ctx context.Context, sentences []string) ([][]float64, error) {
	out := make([][]float64, 0, len(sentences))
	dim := -1
	for start := 0; start < len(sentences); start += e.batchSize {
		end := min(start+e.batchSize, len(sentences))
		vecs, err := e.embedBatch(ctx, sentences[start:end])
		if err != nil {
			return nil, fmt.Errorf("embed sentences %d-%d: %w", start, end-1, err)
		}
		for i, v := range vecs {
			if dim == -1 {
				dim = len(v)
			}
			if len(v) != dim {
				return nil, fmt.Errorf("embedding dimension mismatch at sentence %d: got %d, want %d", start+i, len(v), dim)
			}
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (e *HTTPEmbedder) embedBatch(ctx context.Context, batch []string) ([][]float64, error) {
	body, err := json.Marshal(embeddingRequest{Model: e.model, Input: batch})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		// net/http の *url.Error はリクエスト URL をそのまま含み、パスワード以外の userinfo や
		// クエリの鍵を伏せないので、URL 部分を捨てて原因だけを包む。
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("POST %s: %w", e.displayURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("POST %s: status %d: %s", e.displayURL, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	// 設定先のサーバーが壊れていたり巨大な応答を返したりしてもメモリを食い尽くさないよう上限を設ける。
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(respBody)) > maxResponseBody {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBody)
	}
	var parsed embeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Data) != len(batch) {
		return nil, fmt.Errorf("response has %d embeddings for %d inputs", len(parsed.Data), len(batch))
	}
	vecs := make([][]float64, len(batch))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(batch) {
			return nil, fmt.Errorf("response index %d out of range [0, %d)", d.Index, len(batch))
		}
		if vecs[d.Index] != nil {
			return nil, fmt.Errorf("duplicate response index %d", d.Index)
		}
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("empty embedding at index %d", d.Index)
		}
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}

// redactURL は userinfo（ユーザー名だけのトークンも含む）とクエリ文字列を伏せた URL を返す。
// url.URL.Redacted はパスワードしか伏せないため自前で行う。
func redactURL(u *url.URL) string {
	c := *u
	if c.User != nil {
		c.User = url.User("xxxxx")
	}
	if c.RawQuery != "" {
		c.RawQuery = "xxxxx"
	}
	c.Fragment, c.RawFragment = "", ""
	return c.String()
}
