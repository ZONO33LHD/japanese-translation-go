package outline

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// TemplateHeadingWords はテンプレ見出し語彙のカタログ。リスト順に評価し最初の一致を採用する。
// lint の定型見出し検出より広く、書き出し側の定型（「はじめに」「背景」）も含む。
var TemplateHeadingWords = []string{
	"はじめに", "背景", "概要", "本記事について", "この記事について",
	"まとめと今後", "今後の展望", "今後の課題", "今後について",
	"まとめ", "おわりに", "終わりに", "さいごに", "最後に", "結論", "総括",
	"conclusion", "introduction", "summary",
}

// signaturePOS は見出しの構造パターンを比べるときに残す品詞。
// 助詞や記号を残すと「◯◯の設計」「◯◯の実装」のような対称見出しでもシグネチャがずれるため除く。
var signaturePOS = map[string]bool{"名詞": true, "動詞": true, "形容詞": true, "副詞": true, "接頭辞": true}

var trailingSymbolPOS = map[string]bool{"補助記号": true, "空白": true}

const ws = `[` + pystr.WS + `]`

var (
	// 見出し先頭の番号・記号（「1. はじめに」等）に引きずられずテンプレ語彙を判定するため剥がす
	leadingNumberingRE = regexp.MustCompile(`^[` + pystr.WS + `0-9０-９.．、,()（）【】\[\]#・-]+`)
	numberedHeadingRE  = regexp.MustCompile(`^` + ws + `*(?:[0-9０-９]+[.).、]|[①-⑳])` + ws + `*[^` + pystr.WS + `]`)
	bracketedHeadingRE = regexp.MustCompile(`^` + ws + `*[【\[［(（].+[】\]］)）]` + ws + `*$`)
	towaHeadingRE      = regexp.MustCompile(`^.+とは[?？]?` + ws + `*$`)
)

// TemplateHit はテンプレ見出し語彙に一致した見出し。
type TemplateHit struct {
	Line    int
	Text    string
	Matched string
}

// GroupStats は見出し群 1 つ分の統計。
type GroupStats struct {
	Count                     int
	LengthMean                float64
	LengthCV                  float64
	NominalEndingRatio        float64
	DominantPOSSignatureRatio float64
	TemplateHits              []TemplateHit
	StructuralPatternRatio    float64
}

// LevelStats はレベルごとの統計。レベル昇順に並ぶ。
type LevelStats struct {
	Level int
	Stats GroupStats
}

// HeadingStats は見出し統計全体。
type HeadingStats struct {
	TotalHeadings int
	// LevelDistribution はレベル昇順の (レベル, 本数)。
	LevelDistribution [][2]int
	ByLevel           []LevelStats
	Overall           GroupStats
}

// BuildHeadingStats はスケルトンから見出しだけを取り出し、レベル別と文書全体の統計をまとめる。
func BuildHeadingStats(entries []Entry, tk port.Tokenizer) (HeadingStats, error) {
	var headings []Entry
	byLevel := map[int][]Entry{}
	for _, e := range entries {
		if e.Kind != KindHeading {
			continue
		}
		headings = append(headings, e)
		byLevel[e.Level] = append(byLevel[e.Level], e)
	}
	levels := make([]int, 0, len(byLevel))
	for l := range byLevel {
		levels = append(levels, l)
	}
	slices.Sort(levels)

	stats := HeadingStats{TotalHeadings: len(headings)}
	for _, l := range levels {
		stats.LevelDistribution = append(stats.LevelDistribution, [2]int{l, len(byLevel[l])})
		g, err := summarize(byLevel[l], tk)
		if err != nil {
			return HeadingStats{}, err
		}
		stats.ByLevel = append(stats.ByLevel, LevelStats{Level: l, Stats: g})
	}
	overall, err := summarize(headings, tk)
	if err != nil {
		return HeadingStats{}, err
	}
	stats.Overall = overall
	return stats, nil
}

func summarize(headings []Entry, tk port.Tokenizer) (GroupStats, error) {
	count := len(headings)
	if count == 0 {
		return GroupStats{TemplateHits: []TemplateHit{}}, nil
	}

	total := 0
	lengths := make([]int, count)
	for i, h := range headings {
		lengths[i] = pystr.Len(h.Text)
		total += lengths[i]
	}
	mean := float64(total) / float64(count)
	cv := 0.0
	if mean > 0 && count > 1 {
		variance := 0.0
		for _, l := range lengths {
			d := float64(l) - mean
			variance += d * d
		}
		cv = math.Sqrt(variance/float64(count)) / mean
	}

	nominal := 0
	sigCounts := map[string]int{}
	var sigOrder []string
	for _, h := range headings {
		ms, err := tokenize(tk, h.Text)
		if err != nil {
			return GroupStats{}, err
		}
		if isNominalEnding(ms) {
			nominal++
		}
		if sig := posSignature(ms); sig != "" {
			if sigCounts[sig] == 0 {
				sigOrder = append(sigOrder, sig)
			}
			sigCounts[sig]++
		}
	}
	dominant := 0.0
	if len(sigOrder) > 0 {
		best := 0
		for _, s := range sigOrder {
			best = max(best, sigCounts[s])
		}
		dominant = float64(best) / float64(count)
	}

	hits := []TemplateHit{}
	structural := 0
	for _, h := range headings {
		if w, ok := matchTemplateWord(h.Text); ok {
			hits = append(hits, TemplateHit{Line: h.Line, Text: h.Text, Matched: w})
		}
		if matchStructuralPattern(h.Text) != "" {
			structural++
		}
	}

	return GroupStats{
		Count:                     count,
		LengthMean:                pyfmt.Round(mean, 2),
		LengthCV:                  pyfmt.Round(cv, 3),
		NominalEndingRatio:        pyfmt.Round(float64(nominal)/float64(count), 3),
		DominantPOSSignatureRatio: pyfmt.Round(dominant, 3),
		TemplateHits:              hits,
		StructuralPatternRatio:    pyfmt.Round(float64(structural)/float64(count), 3),
	}, nil
}

func tokenize(tk port.Tokenizer, text string) ([]model.Morpheme, error) {
	if text == "" {
		return nil, nil
	}
	ms, err := tk.Tokenize(text)
	if err != nil {
		return nil, fmt.Errorf("tokenize heading %q: %w", text, err)
	}
	return ms, nil
}

// isNominalEnding は末尾の記号を除いた最終形態素が名詞か（体言止めか）を判定する。
func isNominalEnding(ms []model.Morpheme) bool {
	i := len(ms)
	for i > 0 && trailingSymbolPOS[ms[i-1].POS1()] {
		i--
	}
	return i > 0 && ms[i-1].POS1() == "名詞"
}

func posSignature(ms []model.Morpheme) string {
	var sig []string
	for _, m := range ms {
		if signaturePOS[m.POS1()] {
			sig = append(sig, m.POS1())
		}
	}
	// 区切りに品詞名に現れない文字を使い、タプル比較と同じ意味にする
	return strings.Join(sig, "\x00")
}

func matchTemplateWord(text string) (string, bool) {
	stripped := strings.ToLower(pystr.Strip(leadingNumberingRE.ReplaceAllString(text, "")))
	for _, w := range TemplateHeadingWords {
		if strings.HasPrefix(stripped, strings.ToLower(w)) {
			return w, true
		}
	}
	return "", false
}

// matchStructuralPattern は連番・括弧・「◯◯とは」型のうち最初に一致したものを返す。
func matchStructuralPattern(text string) string {
	switch {
	case numberedHeadingRE.MatchString(text):
		return "numbered"
	case bracketedHeadingRE.MatchString(text):
		return "bracketed"
	case towaHeadingRE.MatchString(text):
		return "towa"
	}
	return ""
}
