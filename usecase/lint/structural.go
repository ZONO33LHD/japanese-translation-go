package lint

import (
	"fmt"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/markdown"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pyfmt"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/sentence"
)

// 構造層検出器の暫定閾値。5 文書の質的観察のみが根拠なので EXPERIMENTAL 扱い。
const (
	boldDensityPer1000Threshold        = 3.0
	bulletLineRatioThreshold           = 0.35
	bulletLineMinLines                 = 10
	numberedPhaseMinCount              = 3
	emojiSymbolPer1000Threshold        = 2.0
	structuralMinSpanCount             = 3
	boilerplateHeadingExcerptMaxLength = 40
)

// StructuralStats は Markdown 構造レベルの集計。
type StructuralStats struct {
	BoldSpanCount           int
	BoldPer1000Chars        float64
	BulletLineCount         int
	NonBlankLineCount       int
	BoilerplateHeadingCount int
	NumberedPhaseHitCount   int
	EmojiSymbolCount        int
	EmojiSymbolPer1000Chars float64
}

func lineOfByteOffset(text string, byteOffset int) int {
	return strings.Count(text[:byteOffset], "\n") + 1
}

// DetectStructuralAIHabits はマスク前のテキスト（HTML コメントのみ空白化済み）に対して、
// 太字密度・箇条書き行比率・定型見出し・番号付きフェーズ・絵文字密度を検出する。
// ここまでの検出器と違い Markdown 構造そのものを見るので、構造マスクより前に呼ぶ。
func DetectStructuralAIHabits(text string) ([]model.Finding, StructuralStats) {
	var findings []model.Finding
	totalChars := max(pystr.Len(text), 1)

	bold := boldSpanRE.FindAllStringIndex(text, -1)
	boldPer1000 := float64(len(bold)) / float64(totalChars) * 1000
	if boldPer1000 >= boldDensityPer1000Threshold && len(bold) >= structuralMinSpanCount {
		findings = append(findings, model.NewFinding(lineOfByteOffset(text, bold[0][0]), "high_bold_density",
			fmt.Sprintf("太字スパン%d箇所（1000字あたり%s）", len(bold), pyfmt.Fixed(boldPer1000, 2)),
			model.SeverityInfo,
			fmt.Sprintf("太字（**...**）の使用密度が閾値（1000字あたり%s）以上。強調の多用は教科書的なAI生成文に見られる傾向（実験的検出器、閾値は暫定）",
				pyfmt.Repr(boldDensityPer1000Threshold)),
			nil))
	}

	lines := sentence.Lines(text)
	var nonBlank int
	var bulletLines []int
	for _, l := range lines {
		if pystr.IsBlank(l.Text) {
			continue
		}
		nonBlank++
		if markdown.ListItemRE.MatchString(l.Text) {
			bulletLines = append(bulletLines, l.No)
		}
	}
	if nonBlank >= bulletLineMinLines {
		ratio := float64(len(bulletLines)) / float64(nonBlank)
		if ratio >= bulletLineRatioThreshold {
			first := 1
			if len(bulletLines) > 0 {
				first = bulletLines[0]
			}
			var related []int
			if len(bulletLines) > 1 {
				related = bulletLines
			}
			findings = append(findings, model.NewFinding(first, "high_bullet_ratio",
				fmt.Sprintf("箇条書き行%d/%d行（%s）", len(bulletLines), nonBlank, pyfmt.Percent(ratio, 1)),
				model.SeverityInfo,
				fmt.Sprintf("箇条書き行の比率が閾値%s以上。文章より箇条書きに頼る構成は教科書的なAI生成文に見られる傾向（実験的検出器）",
					pyfmt.Percent(bulletLineRatioThreshold, 0)),
				capRelated(related)))
		}
	}

	boilerplate := 0
	for _, l := range lines {
		body, ok := markdown.HeadingBody(l.Text)
		if !ok {
			continue
		}
		heading := strings.ToLower(pystr.Strip(body))
		for _, w := range boilerplateHeadingWords {
			if !strings.HasPrefix(heading, strings.ToLower(w)) {
				continue
			}
			boilerplate++
			findings = append(findings, model.NewFinding(l.No, "boilerplate_heading",
				pystr.Head(pystr.Strip(l.Text), boilerplateHeadingExcerptMaxLength),
				model.SeverityInfo,
				"定型見出し「"+w+"」系での締め。予告・構成の型のみで中身を語らない教科書的なAI生成文に見られる傾向（実験的検出器）",
				nil))
			break
		}
	}

	phases := numberedPhaseRE.FindAllStringIndex(text, -1)
	if len(phases) >= numberedPhaseMinCount {
		findings = append(findings, model.NewFinding(lineOfByteOffset(text, phases[0][0]), "numbered_phase_structure",
			fmt.Sprintf("番号付きフェーズ表現が%d回出現", len(phases)),
			model.SeverityInfo,
			fmt.Sprintf("「フェーズ/ステップ/段階+番号」の表現が閾値%d回以上。機械的な段階分割は教科書的なAI生成文に見られる傾向（実験的検出器）",
				numberedPhaseMinCount),
			nil))
	}

	emoji := emojiSymbolRE.FindAllStringIndex(text, -1)
	emojiPer1000 := float64(len(emoji)) / float64(totalChars) * 1000
	if emojiPer1000 >= emojiSymbolPer1000Threshold && len(emoji) >= structuralMinSpanCount {
		findings = append(findings, model.NewFinding(lineOfByteOffset(text, emoji[0][0]), "high_emoji_symbol_density",
			fmt.Sprintf("絵文字/装飾記号%d箇所（1000字あたり%s）", len(emoji), pyfmt.Fixed(emojiPer1000, 2)),
			model.SeverityInfo,
			fmt.Sprintf("絵文字・装飾記号の使用密度が閾値（1000字あたり%s）以上（実験的検出器、閾値は暫定）",
				pyfmt.Repr(emojiSymbolPer1000Threshold)),
			nil))
	}

	return findings, StructuralStats{
		BoldSpanCount:           len(bold),
		BoldPer1000Chars:        boldPer1000,
		BulletLineCount:         len(bulletLines),
		NonBlankLineCount:       nonBlank,
		BoilerplateHeadingCount: boilerplate,
		NumberedPhaseHitCount:   len(phases),
		EmojiSymbolCount:        len(emoji),
		EmojiSymbolPer1000Chars: emojiPer1000,
	}
}
