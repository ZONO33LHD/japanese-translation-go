// Package calibrate はコーパスを使って lint 検出器の閾値を統計的に校正するユースケース。
//
// 人間の文章での誤検知率と AI 生成文での検出率を測り、「人間側 FP 率 5% 未満で
// AI 検出率最大」の閾値を探す。成果物ではなく lint の品質を裏付けるための実験基盤。
package calibrate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// Group はコーパス種別。
type Group string

const (
	HumanAozora   Group = "human_aozora"
	HumanWeb      Group = "human_web"
	HumanBusiness Group = "human_business"
	AI            Group = "ai"
	AIBusiness    Group = "ai_business"
)

// Groups はレポートの列順に並べたコーパス種別。
var Groups = []Group{HumanAozora, HumanWeb, HumanBusiness, AI, AIBusiness}

// Doc はコーパス種別付きの 1 文書。
type Doc struct {
	Group     Group
	Path      string
	Text      string
	CharCount int
}

// Corpus は種別ごとの文書。business の 2 種別は human_web / ai の部分集合で、二重集計ではなく参考列。
type Corpus map[Group][]Doc

// GroupCorpus は読み込んだファイルを種別に振り分ける。
// human 側の business は sources.json の genre、ai 側はファイル名の "business-" 接頭辞で判定する。
func GroupCorpus(files port.CorpusFiles) Corpus {
	c := Corpus{}
	for _, g := range Groups {
		c[g] = nil
	}
	add := func(g Group, d port.CorpusDocument) {
		c[g] = append(c[g], Doc{Group: g, Path: d.Path, Text: d.Text, CharCount: pystr.Len(d.Text)})
	}
	for _, d := range files.Aozora {
		add(HumanAozora, d)
	}
	for _, d := range files.Web {
		add(HumanWeb, d)
		base := filepath.Base(d.Path)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if files.GenreByID[stem] == "business" {
			add(HumanBusiness, d)
		}
	}
	for _, d := range files.AI {
		add(AI, d)
		if strings.HasPrefix(filepath.Base(d.Path), "business-") {
			add(AIBusiness, d)
		}
	}
	return c
}

// Humans は sweep / length-analysis で人間側として扱う文書（aozora + web）。
func (c Corpus) Humans() []Doc { return append(append([]Doc{}, c[HumanAozora]...), c[HumanWeb]...) }

// Prepared は形態素解析まで済ませた文書。sweep で閾値を変えて何度も評価するため解析結果を使い回す。
type Prepared struct {
	Doc      Doc
	Document lint.Document
	Findings []model.Finding
}

// Runner は tokenizer を使って文書の前処理と検出を行う。
type Runner struct {
	tk port.Tokenizer
}

// NewRunner は Runner を作る。
func NewRunner(tk port.Tokenizer) *Runner { return &Runner{tk: tk} }

// Prepare は各文書をマスク・文分割・形態素解析する。
func (r *Runner) Prepare(docs []Doc) ([]Prepared, error) {
	out := make([]Prepared, len(docs))
	for i, d := range docs {
		doc, err := lint.Prepare(r.tk, d.Text)
		if err != nil {
			return nil, fmt.Errorf("prepare %s: %w", d.Path, err)
		}
		out[i] = Prepared{Doc: d, Document: doc}
	}
	return out, nil
}

// PrepareWithFindings は Prepare に加えて既定の閾値で全検出器を実行する（report / length-analysis 用）。
func (r *Runner) PrepareWithFindings(docs []Doc) ([]Prepared, error) {
	ps, err := r.Prepare(docs)
	if err != nil {
		return nil, err
	}
	for i := range ps {
		fs, err := r.fullLint(ps[i].Document)
		if err != nil {
			return nil, fmt.Errorf("lint %s: %w", ps[i].Doc.Path, err)
		}
		ps[i].Findings = fs
	}
	return ps, nil
}

// fullLint は EXPERIMENTAL フィルタもジャンル設定も通さずに全検出器を既定値で実行する。
// 校正にはまさに実験的カテゴリの生の発火率が要るため。構造層は元実装どおり
// HTML コメントを空白化しない原文に対して動かす。
func (r *Runner) fullLint(doc lint.Document) ([]model.Finding, error) {
	fs, _ := lint.Detect(doc, lint.DefaultParams(), lint.DetectOptions{StructuralOnRawText: true})
	return fs, nil
}

func fired(fs []model.Finding, category string) bool {
	for _, f := range fs {
		if f.Category == category {
			return true
		}
	}
	return false
}

func countCategory(fs []model.Finding, category string) int {
	n := 0
	for _, f := range fs {
		if f.Category == category {
			n++
		}
	}
	return n
}
