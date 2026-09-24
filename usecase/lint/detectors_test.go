package lint

import (
	"strings"
	"testing"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// mo は (表層, 品詞大分類, 品詞中分類) から形態素を作り、Begin/End を連結順に振る。
type mo struct {
	surface, pos1, pos2 string
	dict, norm          string
}

func sent(line int, parts ...mo) model.TokenizedSentence {
	var ms []model.Morpheme
	var text strings.Builder
	offset := 0
	for _, p := range parts {
		n := len([]rune(p.surface))
		dict := p.dict
		if dict == "" {
			dict = p.surface
		}
		norm := p.norm
		if norm == "" {
			norm = dict
		}
		ms = append(ms, model.Morpheme{
			Surface: p.surface, POS: [6]string{p.pos1, p.pos2, "*", "*", "*", "*"},
			DictionaryForm: dict, NormalizedForm: norm, Begin: offset, End: offset + n,
		})
		offset += n
		text.WriteString(p.surface)
	}
	return model.TokenizedSentence{Line: line, Text: text.String(), Morphemes: ms, RawText: text.String()}
}

func linesOf(text string) ([]model.Line, map[int]string) {
	masked := markdown.MaskStructure(text)
	return sentence.Lines(masked), sentence.LinesByNo(text)
}

func TestDetectForbiddenPhrases(t *testing.T) {
	lines, raw := linesOf("# このように\n結局、重要なのは中身だと言えるでしょう。\n`このように` はコード。")
	fs := DetectForbiddenPhrases(lines, raw)
	if len(fs) != 2 {
		t.Fatalf("got %d findings: %+v", len(fs), fs)
	}
	if fs[0].Severity != model.SeverityWarn || !strings.Contains(fs[0].Detail, "と言えるでしょう") {
		t.Errorf("first finding = %+v", fs[0])
	}
	if fs[1].Severity != model.SeverityInfo || !strings.Contains(fs[1].Detail, "弱いシグナル") {
		t.Errorf("weak signal finding = %+v", fs[1])
	}
	if fs[1].Excerpt != "結局、重要なのは中身だと言えるでしょ" {
		t.Errorf("excerpt = %q", fs[1].Excerpt)
	}
}

// 一致が行末近くでも excerpt は原文から切り出し、インラインコードの中身を落とさない。
func TestDetectForbiddenPhrasesExcerptKeepsInlineCodeNearLineEnd(t *testing.T) {
	lines, raw := linesOf("`x`を使うことで、非常に重要な点が分かる。")
	fs := DetectForbiddenPhrases(lines, raw)
	if len(fs) != 1 || fs[0].Excerpt != "`x`を使うことで、非常に重要な点が分かる。" {
		t.Fatalf("got %+v", fs)
	}
}

func TestDetectAntithesisRepetitionSeverityByRatio(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		genre    model.Genre
		wantSev  model.Severity
		wantHits int
	}{
		{"below threshold", "AではなくB。\nCではなくD。", model.GenreNone, "", 0},
		{"dense is critical", "AではなくB。\nCではなくD。\nEだけでなくFも。", model.GenreNone, model.SeverityCritical, 3},
		{"sparse is info", "AではなくB。\nCではなくD。\nEではなくF。\n" + strings.Repeat("普通の文。", 200), model.GenreNone, model.SeverityInfo, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := ForGenre(c.genre)
			lines, raw := linesOf(c.text)
			fs := DetectAntithesisRepetition(lines, raw, p)
			if len(fs) != c.wantHits {
				t.Fatalf("got %d findings", len(fs))
			}
			for _, f := range fs {
				if f.Severity != c.wantSev {
					t.Errorf("severity = %s, want %s", f.Severity, c.wantSev)
				}
				if !strings.Contains(f.Detail, "対応箇所: L1") {
					t.Errorf("detail should list related lines: %s", f.Detail)
				}
			}
		})
	}
}

func TestDetectLowSentenceLengthVariance(t *testing.T) {
	uniform := strings.Repeat("これは同じ長さの文です。", 6)
	lines, raw := linesOf(uniform)
	fs := DetectLowSentenceLengthVariance(sentence.SplitWithLines(lines, raw), DefaultParams())
	if len(fs) != 1 {
		t.Fatalf("got %+v", fs)
	}
	if fs[0].Excerpt != "文数=6, 平均文長=11.0字, 変動係数=0.000" {
		t.Errorf("excerpt = %q", fs[0].Excerpt)
	}
	if fs[0].Detail != "文長の変動係数が閾値(0.25)未満。リズムが均質でAI臭い可能性" {
		t.Errorf("detail = %q", fs[0].Detail)
	}
}

func TestDetectEnglishSyntaxCleftBecause(t *testing.T) {
	lines, raw := linesOf("それは必然である。なぜなら市場が変わったからだ。")
	fs := DetectEnglishSyntaxSmell(lines, raw)
	if len(fs) != 1 || fs[0].Category != "english_syntax_cleft_because" {
		t.Fatalf("got %+v", fs)
	}
	if fs[0].Excerpt != "それは必然である。なぜなら市場が変わったからだ" {
		t.Errorf("excerpt = %q", fs[0].Excerpt)
	}
}

func TestDetectTranslationeseMorph(t *testing.T) {
	ts := sent(3,
		mo{"使う", "動詞", "一般", "", ""},
		mo{"こと", "名詞", "普通名詞", "", ""},
		mo{"が", "助詞", "格助詞", "", ""},
		mo{"できる", "動詞", "非自立可能", "", ""},
	)
	fs := DetectTranslationeseMorph([]model.TokenizedSentence{ts})
	if len(fs) != 1 || fs[0].Excerpt != "使うことができる" || fs[0].Line != 3 {
		t.Fatalf("got %+v", fs)
	}
}

func TestDetectInanimateSubjectMorphTwoMorphemeSubject(t *testing.T) {
	ts := sent(1,
		mo{"この", "連体詞", "*", "", ""},
		mo{"事実", "名詞", "普通名詞", "", ""},
		mo{"は", "助詞", "係助詞", "", ""},
		mo{"変化", "名詞", "普通名詞", "", ""},
		mo{"を", "助詞", "格助詞", "", ""},
		mo{"示す", "動詞", "一般", "", ""},
	)
	fs := DetectInanimateSubjectMorph([]model.TokenizedSentence{ts})
	if len(fs) != 1 {
		t.Fatalf("「事実」単体で再マッチして二重検出してはいけない: %+v", fs)
	}
	if !strings.Contains(fs[0].Detail, "抽象主語「この事実」+ は + 他動詞的述語「示す」") {
		t.Errorf("detail = %q", fs[0].Detail)
	}
}

func TestMoraLengthMergesSmallKana(t *testing.T) {
	ms := []model.Morpheme{{Surface: "今日", ReadingForm: "キョウ"}, {Surface: "は", ReadingForm: "ハ"}}
	if got := moraLength(ms); got != 3 {
		t.Errorf("moraLength = %d, want 3", got)
	}
}

func TestComputeMTLD(t *testing.T) {
	if computeMTLD(strings.Fields("a b c"), 0.72) != nil {
		t.Error("MTLD must be nil for fewer than 20 tokens")
	}
	same := strings.Fields(strings.Repeat("x ", 30))
	got := computeMTLD(same, 0.72)
	if got == nil || *got <= 0 || *got > 2 {
		t.Errorf("MTLD of a single repeated token should be tiny, got %v", got)
	}
}

func TestIsConditionalNegation(t *testing.T) {
	cases := map[string]bool{
		"ないと動か":  true,
		"なければ意味": true,
		"なくば":    true,
		"ないとは言え": false,
		"ないわけでは": false,
	}
	for span, want := range cases {
		if got := isConditionalNegation(span); got != want {
			t.Errorf("isConditionalNegation(%q) = %v, want %v", span, got, want)
		}
	}
}

func TestReadingLengthCollapsesMaskedWhitespace(t *testing.T) {
	if got := readingLength("リンク(          )を見る"); got != 9 {
		t.Errorf("readingLength = %d, want 9", got)
	}
}

func TestSegmentEndsWithNounSkipsParenGroup(t *testing.T) {
	seg := sent(1,
		mo{"確認", "名詞", "普通名詞", "", ""},
		mo{"（", "補助記号", "括弧開", "", ""},
		mo{"出る", "動詞", "一般", "", ""},
		mo{"か", "助詞", "終助詞", "", ""},
		mo{"）", "補助記号", "括弧閉", "", ""},
	).Morphemes
	if !segmentEndsWithNoun(seg) {
		t.Error("括弧グループの手前の名詞で終わる区画とみなすべき")
	}
}

func TestDetectReadingLoadDoubleNegativeAndObligation(t *testing.T) {
	litotes := sent(1,
		mo{"招か", "動詞", "一般", "招く", ""},
		mo{"ない", "助動詞", "*", "", "ない"},
		mo{"と", "助詞", "格助詞", "", ""},
		mo{"は", "助詞", "係助詞", "", ""},
		mo{"言え", "動詞", "一般", "言える", ""},
		mo{"ませ", "助動詞", "*", "ます", ""},
		mo{"ん", "助動詞", "*", "ぬ", "ず"},
	)
	obligation := sent(2,
		mo{"保た", "動詞", "一般", "保つ", ""},
		mo{"ない", "助動詞", "*", "", "ない"},
		mo{"と", "助詞", "接続助詞", "", ""},
		mo{"いけ", "動詞", "非自立可能", "いける", ""},
		mo{"ませ", "助動詞", "*", "ます", ""},
		mo{"ん", "助動詞", "*", "ぬ", "ず"},
	)
	fs := DetectReadingLoad([]model.TokenizedSentence{litotes, obligation}, 90)
	if len(fs) != 1 || fs[0].Category != "double_negative" || fs[0].Line != 1 || fs[0].Excerpt != "ないとは言えません" {
		t.Fatalf("got %+v", fs)
	}
}

func TestDetectStructuralAIHabits(t *testing.T) {
	text := "# 設計\n**A** と **B** と **C**。\nフェーズ1、フェーズ2、フェーズ3。\n## まとめ\n"
	fs, stats := DetectStructuralAIHabits(text)
	cats := map[string]int{}
	for _, f := range fs {
		cats[f.Category] = f.Line
	}
	if cats["high_bold_density"] != 2 || cats["numbered_phase_structure"] != 3 || cats["boilerplate_heading"] != 4 {
		t.Errorf("findings = %+v", fs)
	}
	if stats.BoldSpanCount != 3 || stats.NumberedPhaseHitCount != 3 || stats.BoilerplateHeadingCount != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestFilterCategories(t *testing.T) {
	fs := []model.Finding{
		{Category: "forbidden_phrase"},
		{Category: "high_bold_density"},
		{Category: "repeated_syntax_template"},
	}
	if got := FilterCategories(fs, false, nil); len(got) != 1 {
		t.Errorf("default must drop experimental categories: %+v", got)
	}
	_, disabled := ForGenre(model.GenreBusiness)
	if got := FilterCategories(fs, true, disabled); len(got) != 2 {
		t.Errorf("business must drop high_bold_density even with experimental: %+v", got)
	}
}

func splitForTest(lines []model.Line, raw map[int]string) []model.Sentence {
	return sentence.SplitWithLines(lines, raw)
}
