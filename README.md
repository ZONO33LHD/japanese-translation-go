# japanese-translation-go

仕事の日本語文書から「AIっぽさ」と読みにくさを拾い出す CLI `natural-japanese` と、それを使う Agent Skill です。

[coji/natural-japanese](https://github.com/coji/natural-japanese)（MIT）の Go 移植です。元リポジトリでは Python（sudachipy）で書かれていた検査スクリプト群を、Go の単一バイナリに置き換えました。文体憲法・禁止パターン・推敲ルーブリックなどのスキル文書は `skills/natural-japanese/` に元リポジトリから引き継いでおり、スクリプトの呼び出し部分だけを Go 版の CLI に書き換えています。

検出の考え方は元実装と同じで、「検出は機械、判断は人間（またはエージェント）」です。CLI は疑わしい箇所を決定的に指さすだけで、直すかどうかは文脈を読んで決めます。

## インストール

Go 1.27.1 以上が必要です（`GOTOOLCHAIN=auto` なら古い Go からでも自動で取得されます）。

```sh
go install github.com/ZONO33LHD/japanese-translation-go/cmd/natural-japanese@latest
```

### 辞書の用意

形態素解析には [sudachin-go](https://github.com/ZONO33LHD/sudachin-go)（sudachi.rs と出力互換の Go 実装）を使います。SudachiDict core のシステム辞書を取得してください。

```sh
curl -LO https://d2ej7fkh96fzlu.cloudfront.net/sudachidict/sudachi-dictionary-20260723-core.zip
echo "b6e835f63440f97474c2da45d80950f73746e632e40bbfc168b4041729135e1f  sudachi-dictionary-20260723-core.zip" | shasum -a 256 -c -
unzip -j sudachi-dictionary-20260723-core.zip '*.dic'
export SUDACHIN_DICT=$PWD/system_core.dic
```

検出件数の回帰テストはこの版（20260723）で確認しています。別の版でも動きますが、形態素の切り方が変わると検出結果も変わることがあります。

辞書のパスは各サブコマンドの `--dict` でも渡せます。`--dict` が優先で、未指定なら `$SUDACHIN_DICT` を読みます。

## 使い方

```sh
natural-japanese <command> [options] <file>
```

フラグとファイル名の順序は自由です（`lint draft.md --json` と `lint --json draft.md` は同じ）。

### lint — AI臭い表現の検出

```sh
natural-japanese lint draft.md
natural-japanese lint draft.md --json
natural-japanese lint draft.md --genre tech
natural-japanese lint draft.md --reading-load
natural-japanese lint draft.md --json --baseline prev.json
```

| フラグ | 意味 |
|---|---|
| `--json` | 機械可読な JSON で出力する |
| `--genre essay\|tech\|business` | コーパス校正済みのジャンル別閾値を使う |
| `--experimental` | 定量校正前の実験的検出器（構造層など）も出力する |
| `--reading-load` | 読解負荷レーン（一文長・埋もれた列挙・連続漢字・二重否定・「の」連鎖）を別枠で出す。スコアや baseline 比較には入らない |
| `--baseline PREV.json` | 前回の `--json` 出力と比べ、resolved / new / persisting に仕分ける |
| `--dict PATH` | Sudachi システム辞書 |

禁止語・翻訳調・否定→肯定対比の反復・文長の均質さ・体言止めの欠如・語彙多様性・具体性の不足・英語統語の直訳調などを検出します。

### outline — スケルトンと見出し統計

```sh
natural-japanese outline draft.md [--json] [--dict PATH]
```

見出し・各段落の先頭文・箇条書きのプレースホルダを行番号付きで抜き出し、見出しの長さ・体言止め率・品詞パターンの一致率・テンプレ見出しのヒットを集計します。統計は判断材料で、良し悪しは判定しません。

### terms — 専門用語候補

```sh
natural-japanese terms draft.md [--json] [--dict PATH]
```

カタカナ複合語・ASCII 略語・固有名詞らしき語を初出順に並べ、出現回数と初出近傍、説明の手掛かり（`has_gloss_hint`）を出します。

### semantic — 話題の平板さ（EXPERIMENTAL）

```sh
natural-japanese semantic draft.md [--json] [--genre essay|tech|business] \
  [--endpoint URL] [--model cl-nagoya/ruri-v3-310m] [--api-key KEY]
```

文埋め込みで隣接文の類似度の起伏を測ります。埋め込みモデルは同梱していません。cl-nagoya/ruri-v3-310m を OpenAI 互換の `/v1/embeddings` API で配信するサーバー（text-embeddings-inference、Ollama、llama.cpp、vLLM など）を別途立て、`--endpoint` か `$NJ_EMBEDDING_ENDPOINT` で指定してください（既定は `http://localhost:8080`）。API キーが必要なら `$NJ_EMBEDDING_API_KEY` で渡してください（`--api-key` も使えますが、プロセス一覧やシェル履歴に残ります）。10 文未満の文書では測定を省きます。

文書の各文をそのままエンドポイントへ送ります。社外のサーバーを指定するときは送ってよい文書か確かめてください。`http://` で localhost・プライベートアドレス以外を指定すると、平文で送る旨の警告を出します。

埋め込みサーバーの実装（プーリング・量子化・切り詰め）によって類似度が変わり、sentence-transformers で校正した閾値が合わなくなる可能性があります。実サーバーでの検証はまだです。

### calibrate — 閾値の校正（開発者向け）

```sh
natural-japanese calibrate report --corpus <dir>
natural-japanese calibrate sweep --detector low_burstiness --corpus <dir>
natural-japanese calibrate length-analysis --corpus <dir>
```

人間の文章と AI の文章を集めたコーパスで検出器の発火率を測り、閾値を探索します。コーパスは同梱していないので、元リポジトリの `corpus/` を取得して `--corpus` に渡してください。コーパスに文書が 1 件もなければ exit 1、読み飛ばしたファイルは警告として stderr に出します。

### 終了コード

| コード | 意味 |
|---|---|
| 0 | 実行できた。検出件数に関わらず 0（lint であって CI ゲートではない） |
| 1 | 入力エラー（ファイルがない、ディレクトリを指定した、UTF-8 でない、辞書や埋め込み API が使えない等） |
| 2 | 引数の誤り |

## 構成

オニオンアーキテクチャで組んでいます。依存は常に外側から内側へ向かいます（infrastructure / interface → usecase → domain）。

```
cmd/natural-japanese/        コンポジションルート（具象実装を組み立てて CLI に注入する）
domain/
  model/                     Finding・Morpheme・文・行などのエンティティと値オブジェクト
  service/
    markdown/                Markdown 構造の判定とマスク（行番号・オフセットを保つ）
    sentence/                行・段落・文への分割
    pystr/                   元実装の str と同じ意味論の文字列操作（ルーン単位）
    pyfmt/                   元実装と同じ数値表記（float repr・%表記・round）
usecase/
  port/                      Tokenizer・Embedder・SourceReader のインターフェース
  lint/                      lint 検出器群・読解負荷レーン・baseline 比較
  outline/                   スケルトン抽出と見出し統計
  terms/                     専門用語インベントリ
  semantic/                  文埋め込みによる平板さ検出
  calibrate/                 コーパス校正
interface/
  cli/                       サブコマンドの引数解釈（コントローラ）
  presenter/                 人間可読テキストと JSON の出力
infrastructure/
  sudachi/                   sudachin-go による Tokenizer 実装
  embedding/                 OpenAI 互換 HTTP API による Embedder 実装
  filesystem/                ファイル読み込み
skills/natural-japanese/     Agent Skill 本体（SKILL.md・references・assets）
```

ユースケース層は `usecase/port` のインターフェースだけを知っていて、Sudachi 辞書や埋め込みサーバーの存在を知りません。テストでは偽の Tokenizer / Embedder を差し込めます。

## テスト

```sh
make test        # go test -race ./...（辞書が必要なテストはスキップ）
make test-dict   # 辞書を .cache/ に取得してから全テスト
make vet lint    # go vet と staticcheck
```

`$CI` が設定されていると、辞書が無いときにスキップせず失敗します（CI で回帰テストが黙って抜けないようにするため）。GitHub Actions（`.github/workflows/ci.yml`）は辞書をキャッシュして gofmt・vet・staticcheck・govulncheck・race 付きテストを回します。

## 元実装との違い

検出器・閾値・出力形式は元実装に合わせていますが、次の点は意図して変えています。

- 文頭がインラインコードの文で、品詞列マッチの excerpt がずれて壊れていたのを直した
- 禁止語などの excerpt が行末近くでマスク済みテキスト（コード部分が空白）から切り出されていたのを、常に原文から切り出すようにした
- 行は LF だけで区切る（元実装は U+2028 などでも区切り、outline やエディタと行番号がずれていた）
- `mora_mean` などの統計値は常に小数で出す。NaN・無限大は null にする
- 反復系の finding が持つ対応箇所（`related_lines` と detail の行番号一覧）は先頭 20 件までにした。全 finding が全行番号を持つと、反復の多い文書で出力が件数の 2 乗で膨らむため
- 品詞列マッチの excerpt は、マスクで空白になった区間（インラインコード・リンク URL）の途中から切り出さない。80 文字を超える excerpt は切り詰める
- 入力ファイルは 20 MiB まで。人間向け出力では制御文字を `\xNN` に置き換える（JSON は変えない）

検出件数は元リポジトリの回帰チェックの期待値（ai-smelly.md 25/33 件、natural.md 0/0 件）と一致します。バイト単位で元実装の出力と突き合わせる golden テストはまだありません。

## スキルとして使う

`skills/natural-japanese/` をエージェントのスキルディレクトリに置き、`natural-japanese` コマンドに PATH を通してください。手順は `skills/natural-japanese/SKILL.md` に書いてあります。CLI を実行できない環境では `references/manual-checklist.md` を目視で当てます。

スキル文書の中にある `corpus/reports/...` へのリンクは、元リポジトリの校正レポートを指しています（このリポジトリには含めていません）。

## ライセンスとクレジット

MIT License。スキル文書と検出ロジックは coji さんの [natural-japanese](https://github.com/coji/natural-japanese) に由来します（詳細は [NOTICE](NOTICE)）。元リポジトリが参考資料として挙げているものも、そのまま記しておきます。

- [AI臭さを消した日本語執筆エージェントの設計（なつ「いとおり」）](https://note.com/art_reflection/n/n7ffd5ce3320c)
- [日本語技術文書の文章規範（k16shikano）](https://gist.github.com/k16shikano/fd287c3133457c4fd8f5601d34aa817d)
- [meiseki（bamboo-nova）](https://github.com/bamboo-nova/meiseki)
