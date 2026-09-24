// Package lint は AI 臭い日本語文章を決定的に検出するユースケース。
//
// 「AI は自分自身の AI 臭さを認識できない」ため、機械的に検出して人間（または別セッションの
// AI）に突きつけ、直すかどうかの判断は委ねる。CI ゲートではないので件数で失敗させない。
package lint

import (
	"fmt"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/port"
)

// Document は検出器が共有する解析済みの文書。形態素解析は文ごとに 1 回だけ行い使い回す。
type Document struct {
	Raw string
	// Lines は Markdown 構造をマスクした行（解析用）。
	Lines []model.Line
	// RawLines は原文の行（excerpt 表示用）。
	RawLines  map[int]string
	Sentences []model.Sentence
	Tokenized []model.TokenizedSentence
}

// Prepare は原文をマスク・文分割・形態素解析して Document を作る。
func Prepare(tk port.Tokenizer, raw string) (Document, error) {
	masked := markdown.MaskStructure(raw)
	lines := sentence.Lines(masked)
	rawLines := sentence.LinesByNo(raw)
	sentences := sentence.SplitWithLines(lines, rawLines)
	tokenized, err := tokenizeSentences(tk, sentences)
	if err != nil {
		return Document{}, err
	}
	return Document{Raw: raw, Lines: lines, RawLines: rawLines, Sentences: sentences, Tokenized: tokenized}, nil
}

func tokenizeSentences(tk port.Tokenizer, sentences []model.Sentence) ([]model.TokenizedSentence, error) {
	out := make([]model.TokenizedSentence, 0, len(sentences))
	for _, s := range sentences {
		if s.Masked == "" {
			continue
		}
		ms, err := tk.Tokenize(s.Masked)
		if err != nil {
			return nil, fmt.Errorf("tokenize sentence at line %d: %w", s.Line, err)
		}
		raw, offset := s.Raw, s.RawOffset
		if raw == "" {
			raw, offset = s.Masked, 0
		}
		out = append(out, model.TokenizedSentence{Line: s.Line, Text: s.Masked, Morphemes: ms, RawText: raw, RawOffset: offset})
	}
	return out, nil
}

// CategoryCount はカテゴリ別の件数。並びは初出順（元実装の dict の挿入順）。
type CategoryCount struct {
	Category string
	Count    int
}

// CountByCategory は findings をカテゴリ別に数える。
func CountByCategory(fs []model.Finding) []CategoryCount {
	idx := map[string]int{}
	var out []CategoryCount
	for _, f := range fs {
		i, ok := idx[f.Category]
		if !ok {
			idx[f.Category] = len(out)
			out = append(out, CategoryCount{f.Category, 1})
			continue
		}
		out[i].Count++
	}
	return out
}

// Stats は lint 全体の集計。
type Stats struct {
	TotalFindings    int
	ByCategory       []CategoryCount
	Genre            model.Genre
	Experimental     bool
	Morph            MorphStats
	Rhythm           *RhythmStats
	Ngram            NgramStats
	LexicalDiversity LexicalDiversityStats
	Structural       StructuralStats
	LowSpecificity   LowSpecificityStats
}

// Options は lint の実行オプション。
type Options struct {
	Genre        model.Genre
	Experimental bool
}

// Service は lint ユースケースの入口。
type Service struct {
	tk port.Tokenizer
}

// NewService は Tokenizer を注入して Service を作る。
func NewService(tk port.Tokenizer) *Service { return &Service{tk: tk} }

// Prepare は原文を解析して Document を作る。Lint と ReadingLoad で同じ Document を使い回し、
// 形態素解析を 1 回で済ませるために公開している。
func (s *Service) Prepare(raw string) (Document, error) { return Prepare(s.tk, raw) }

// Run は Prepare と Lint をまとめて行う。
func (s *Service) Run(raw string, opt Options) ([]model.Finding, Stats, error) {
	doc, err := s.Prepare(raw)
	if err != nil {
		return nil, Stats{}, err
	}
	fs, stats := Lint(doc, opt)
	return fs, stats, nil
}

// Lint はジャンル別プロファイルを適用して全検出器を実行する。
func Lint(doc Document, opt Options) ([]model.Finding, Stats) {
	params, disabled := ForGenre(opt.Genre)
	findings, stats := Detect(doc, params, DetectOptions{})
	findings = FilterCategories(findings, opt.Experimental, disabled)
	stats.Genre = opt.Genre
	stats.Experimental = opt.Experimental
	stats.TotalFindings = len(findings)
	stats.ByCategory = CountByCategory(findings)
	return findings, stats
}

// DetectOptions は Detect の挙動の切り替え。
type DetectOptions struct {
	// StructuralOnRawText は構造層検出器に HTML コメントを空白化しない原文を渡す。
	// 元実装の calibrate がこの経路で校正しているため、calibrate からだけ使う。
	StructuralOnRawText bool
}

// Detect は解析済みの文書に全検出器をかける（EXPERIMENTAL やジャンルによる除外はしない）。
// lint と calibrate が同じ検出器一覧を共有するための唯一の入口。
func Detect(doc Document, p Params, opt DetectOptions) ([]model.Finding, Stats) {
	// 構造層は Markdown 構造そのものを見るので、構造マスク前のテキストに対して動かす。
	structuralInput := markdown.MaskHTMLComments(doc.Raw)
	if opt.StructuralOnRawText {
		structuralInput = doc.Raw
	}
	structural, structuralStats := DetectStructuralAIHabits(structuralInput)

	var fs []model.Finding
	fs = append(fs, structural...)
	fs = append(fs, DetectForbiddenPhrases(doc.Lines, doc.RawLines)...)
	fs = append(fs, DetectTranslationese(doc.Lines, doc.RawLines)...)
	fs = append(fs, DetectAntithesisRepetition(doc.Lines, doc.RawLines, p)...)
	fs = append(fs, DetectLowSentenceLengthVariance(doc.Sentences, p)...)
	fs = append(fs, DetectEnglishSyntaxSmell(doc.Lines, doc.RawLines)...)

	morphFindings, morphStats := DetectNominalEndingAndParagraphConjunctions(doc.Lines, doc.Tokenized, doc.RawLines, p)
	fs = append(fs, morphFindings...)
	fs = append(fs, DetectTranslationeseMorph(doc.Tokenized)...)
	fs = append(fs, DetectInanimateSubjectMorph(doc.Tokenized)...)

	rhythmFindings, rhythmStats := DetectRhythmStatistics(doc.Tokenized, p)
	fs = append(fs, rhythmFindings...)
	ngramFindings, ngramStats := DetectNgramRepetition(doc.Tokenized, p)
	fs = append(fs, ngramFindings...)
	lexFindings, lexStats := DetectLexicalDiversity(doc.Tokenized, p)
	fs = append(fs, lexFindings...)
	specFindings, specStats := DetectLowSpecificity(doc.Lines, doc.Tokenized, doc.RawLines, p)
	fs = append(fs, specFindings...)

	return model.SortFindingsByLine(fs), Stats{
		Morph:            morphStats,
		Rhythm:           rhythmStats,
		Ngram:            ngramStats,
		LexicalDiversity: lexStats,
		Structural:       structuralStats,
		LowSpecificity:   specStats,
	}
}

// FilterCategories は EXPERIMENTAL カテゴリ（experimental=false のとき）とジャンルで無効化した
// カテゴリを除く。--experimental 指定時でも、ジャンルの正当な慣習と衝突する検出器は必ず除く。
func FilterCategories(fs []model.Finding, experimental bool, disabled map[string]bool) []model.Finding {
	out := make([]model.Finding, 0, len(fs))
	for _, f := range fs {
		if !experimental && ExperimentalCategories[f.Category] {
			continue
		}
		if disabled[f.Category] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// ReadingLoadStats は読解負荷レーンの集計。
type ReadingLoadStats struct {
	Total      int
	Sentences  int
	Genre      model.Genre
	ByCategory []CategoryCount
}

// ReadingLoad は読解負荷レーンだけを実行する。Lint と別関数にしてあるのは、
// AI臭さの findings / stats / baseline に読解負荷の指摘が混ざる経路を構造的に作らないため。
func ReadingLoad(doc Document, genre model.Genre) ([]model.Finding, ReadingLoadStats) {
	params, _ := ForGenre(genre)
	fs := DetectReadingLoad(doc.Tokenized, params.ReadingLoadSentenceMaxChars)
	return fs, ReadingLoadStats{
		Total:      len(fs),
		Sentences:  len(doc.Tokenized),
		Genre:      genre,
		ByCategory: CountByCategory(fs),
	}
}
