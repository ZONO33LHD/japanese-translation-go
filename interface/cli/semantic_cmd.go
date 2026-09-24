package cli

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/interface/presenter"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/semantic"
)

// 埋め込みバックエンドの既定値を与える環境変数。
const (
	EmbeddingEndpointEnv = "NJ_EMBEDDING_ENDPOINT"
	EmbeddingAPIKeyEnv   = "NJ_EMBEDDING_API_KEY"
	defaultEmbeddingURL  = "http://localhost:8080"
)

func semanticGenreChoices() []string {
	out := make([]string, len(model.Genres))
	for i, g := range model.Genres {
		out[i] = string(g)
	}
	return out
}

func semanticEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runSemantic(ctx context.Context, app *App, args []string) int {
	fs := app.newFlagSet("semantic", "[--json] [--genre essay|tech|business] [--model NAME] [--endpoint URL] [--api-key KEY] <file>")
	asJSON := fs.Bool("json", false, "機械可読な JSON で出力する")
	genre := &choiceFlag{choices: semanticGenreChoices()}
	fs.Var(genre, "genre", "ジャンル別に校正した閾値プロファイルを適用する（business/essay/tech）。未指定時は共通閾値")
	modelName := fs.String("model", semantic.DefaultModel, "埋め込みモデル名（閾値はこのモデルで校正済み）")
	// 環境変数の値をフラグの既定値に入れると -h やフラグエラー時の usage に平文で表示されるため、
	// 既定値は空にしておき、解析後に環境変数へフォールバックする。
	endpoint := fs.String("endpoint", "",
		"OpenAI 互換 /v1/embeddings を提供するサーバーの URL（省略時は $"+EmbeddingEndpointEnv+"、それもなければ "+defaultEmbeddingURL+"）")
	apiKey := fs.String("api-key", "",
		"埋め込み API の Bearer トークン。ps やシェル履歴に残るため $"+EmbeddingAPIKeyEnv+" での指定を推奨")

	path, code, ok := app.parseSingleFile(fs, args)
	if !ok {
		return code
	}
	if *endpoint == "" {
		*endpoint = semanticEnvOr(EmbeddingEndpointEnv, defaultEmbeddingURL)
	}
	if *apiKey == "" {
		*apiKey = os.Getenv(EmbeddingAPIKeyEnv)
	}
	displayEndpoint, ok := validateEndpoint(*endpoint)
	if !ok {
		fmt.Fprintf(app.Stderr, "エラー: 引数が正しくありません: --endpoint は http:// か https:// で始まる URL を指定してください: %s\n", displayEndpoint)
		fs.Usage()
		return ExitUsage
	}
	text, err := app.Reader.Read(path)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}

	embedder, err := app.NewEmbedder(EmbedderConfig{Endpoint: *endpoint, Model: *modelName, APIKey: *apiKey})
	if err != nil {
		fmt.Fprintf(app.Stderr, "エラー: 意味モデルの読み込みまたは推論に失敗しました: %v\n", err)
		return ExitInputError
	}
	notifying := &notifyingEmbedder{Embedder: embedder, stderr: app.Stderr, endpoint: *endpoint, display: displayEndpoint}

	findings, stats, err := semantic.RunSemantic(ctx, text, model.Genre(genre.value), notifying)
	if err != nil {
		fmt.Fprintf(app.Stderr, "エラー: 意味モデルの読み込みまたは推論に失敗しました: %v\n", err)
		return ExitInputError
	}

	if *asJSON {
		if err := presenter.WriteSemanticJSON(app.Stdout, path, findings, stats); err != nil {
			fmt.Fprintf(app.Stderr, "エラー: JSON を書き出せません: %v\n", err)
			return ExitInputError
		}
	} else {
		presenter.WriteSemanticHuman(app.Stdout, path, findings, stats)
	}
	// 文章の中身に関する判定は件数に関わらず exit 0（lint と同じ規律）。
	return ExitOK
}

// notifyingEmbedder は実際にバックエンドへ問い合わせる直前に一度だけ stderr へ知らせる。
// 短文書でスキップされる場合は問い合わせ自体が起きないので、通知も出さない。
type notifyingEmbedder struct {
	port.Embedder
	stderr   io.Writer
	endpoint string
	// display は表示用の URL（userinfo のパスワードを伏せたもの）。
	display string
	once    sync.Once
}

func (n *notifyingEmbedder) Embed(ctx context.Context, sentences []string) ([][]float64, error) {
	n.once.Do(func() {
		fmt.Fprintf(n.stderr,
			"[semantic] 埋め込みAPIに問い合わせ中: %s（モデル: %s、%d文。文書の各文をこのサーバーへ送信します。"+
				"OpenAI互換の /v1/embeddings を提供するサーバーが起動している必要があります）\n",
			n.display, n.ModelName(), len(sentences))
		if isCleartextRemote(n.endpoint) {
			fmt.Fprintf(n.stderr,
				"[semantic] 警告: %s は暗号化されていない http の外部ホストです。文書の内容と API キーが平文でネットワークを流れます。https を使ってください。\n",
				n.display)
		}
	})
	return n.Embedder.Embed(ctx, sentences)
}

// validateEndpoint は --endpoint が絶対 http(s) URL かを検査し、表示用の URL を返す。
// URL に認証情報が含まれうるので、表示には redactURL で伏せた形だけを使い、
// 解釈できない値（scheme のない "user:token@host" など）は一切表示しない。
func validateEndpoint(endpoint string) (display string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || u.Host == "" {
		return "（http:// または https:// で始まる URL を指定してください）", false
	}
	return redactURL(u), true
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

// isCleartextRemote は http（非 TLS）で、ループバックでもプライベートアドレスでもないホストかを判定する。
// ローカルの埋め込みサーバーは http が普通なので、外部へ平文で出るときだけ警告するため。
// リンクローカル（169.254.0.0/16, fe80::/10）はクラウドのメタデータサービスが置かれる帯なので
// 安全側には含めない。
func isCleartextRemote(endpoint string) bool {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || !strings.EqualFold(u.Scheme, "http") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified())
	}
	return true
}
