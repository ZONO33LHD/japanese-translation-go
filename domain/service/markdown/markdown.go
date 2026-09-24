// Package markdown は Markdown 構造の判定とマスク処理を提供する。
//
// 見出し・リスト・コードブロック・引用・表・YAML フロントマターは「文章」ではないため、
// 文体の検出器から外す。行を削除すると後続の行番号がずれるので、行の中身を空文字に
// 置き換えるマスク方式で行番号とオフセットを保つ。
package markdown

import (
	"regexp"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

const ws = `[` + pystr.WS + `]`

var (
	HeadingRE          = regexp.MustCompile(`^` + ws + `*#{1,6}(?:` + ws + `|$)`)
	ListItemRE         = regexp.MustCompile(`^` + ws + `*(?:[-*+]|\p{Nd}+[.)])(?:` + ws + `|$)`)
	BlockquoteRE       = regexp.MustCompile(`^` + ws + `*>`)
	CodeFenceRE        = regexp.MustCompile("^" + ws + "*(`{3,}|~{3,})")
	TableRowRE         = regexp.MustCompile(`^` + ws + `*\|.*\|`)
	TableDelimiterRE   = regexp.MustCompile(`^` + ws + `*\|?[` + pystr.WS + `:|-]+\|[` + pystr.WS + `:|-]*\|?` + ws + `*$`)
	FrontMatterDelimRE = regexp.MustCompile(`^---` + ws + `*$`)
	markdownLinkURLRE  = regexp.MustCompile(`(\]\()([^)]*)(\))`)
	headingPartsRE     = regexp.MustCompile(`^` + ws + `*(#{1,6})` + ws + `*(.*?)(?:` + ws + `+#+)?` + ws + `*$`)
)

// IsTableLine は表の行（`|` 始まりで `|` を 2 個以上含む行、または区切り行）かを判定する。
// 本文中にたまたま `|` が 1 個だけ出るケースを誤マスクしないよう保守的に判定する。
func IsTableLine(line string) bool {
	return (TableRowRE.MatchString(line) && strings.Count(line, "|") >= 2) || TableDelimiterRE.MatchString(line)
}

// Fence は開いているコードフェンスの種類と長さ。
type Fence struct {
	Char rune
	Len  int
}

// FenceLine はフェンス行なら (フェンス, 閉じフェンスになりうるか, true) を返す。
// 閉じフェンスは「フェンス文字の連続＋後続は空白のみ」の行に限る（CommonMark に合わせ、
// フェンス内の地の文がたまたま ``` で始まるだけの行でクローズしないため）。
func FenceLine(line string) (Fence, bool, bool) {
	loc := CodeFenceRE.FindStringSubmatchIndex(line)
	if loc == nil {
		return Fence{}, false, false
	}
	run := line[loc[2]:loc[3]]
	f := Fence{Char: rune(run[0]), Len: len(run)}
	return f, pystr.IsBlank(line[loc[1]:]), true
}

// Closes は開いているフェンス open を line のフェンス f が閉じるかを返す。
// 同じ文字種かつ開始フェンス以上の長さのときだけ閉じる。
func (open Fence) Closes(f Fence, closeEligible bool) bool {
	return f.Char == open.Char && f.Len >= open.Len && closeEligible
}

// maskHTMLCommentsInLine は行内の HTML コメントを同じ長さの空白に置換する。
// 複数行コメントに対応するため、コメント内かどうかの状態を受け取り更新後の状態を返す。
// 閉じタグがないまま行末に達したら、行末までを空白化してコメント継続のまま返す。
func maskHTMLCommentsInLine(line string, inComment bool) (string, bool) {
	var b strings.Builder
	rest := line
	for rest != "" {
		if inComment {
			idx := strings.Index(rest, "-->")
			if idx < 0 {
				b.WriteString(strings.Repeat(" ", pystr.Len(rest)))
				rest = ""
				break
			}
			end := idx + len("-->")
			b.WriteString(strings.Repeat(" ", pystr.Len(rest[:end])))
			rest = rest[end:]
			inComment = false
			continue
		}
		idx := strings.Index(rest, "<!--")
		if idx < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:idx])
		rest = rest[idx:]
		inComment = true
	}
	return b.String(), inComment
}

// MaskHTMLComments は HTML コメントだけを同じ長さの空白に置換したテキストを返す。
// Markdown 構造そのものを見る検出器が、コメント内の誤検知だけを避けるために使う。
func MaskHTMLComments(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, len(lines))
	in := false
	for i, line := range lines {
		out[i], in = maskHTMLCommentsInLine(line, in)
	}
	return strings.Join(out, "\n")
}

// blankInlineCodeSpans はインラインコードスパンと Markdown リンク/画像の URL 部分を
// 同じ文字数の空白に置換する（alt/text 側は文章として残す）。
func blankInlineCodeSpans(line string) string {
	line = blankCodeSpans(line)
	var b strings.Builder
	last := 0
	for _, m := range markdownLinkURLRE.FindAllStringSubmatchIndex(line, -1) {
		b.WriteString(line[last:m[4]])
		b.WriteString(strings.Repeat(" ", pystr.Len(line[m[4]:m[5]])))
		last = m[5]
	}
	b.WriteString(line[last:])
	return b.String()
}

// blankCodeSpans はバッククォート 1〜2 個で囲んだインラインコードを空白化する。
// 元実装の次の正規表現は否定先読みを含み RE2 で表現できないため、同じマッチを返す走査として実装している。
//
//	``(?:[^`\n]|`(?!`))+``|`[^`\n]+`
func blankCodeSpans(line string) string {
	rs := []rune(line)
	out := make([]rune, 0, len(rs))
	i := 0
	for i < len(rs) {
		if end, ok := matchCodeSpan(rs, i); ok {
			for range end - i {
				out = append(out, ' ')
			}
			i = end
			continue
		}
		out = append(out, rs[i])
		i++
	}
	return string(out)
}

func matchCodeSpan(rs []rune, i int) (int, bool) {
	if rs[i] != '`' {
		return 0, false
	}
	n := len(rs)
	if i+1 < n && rs[i+1] == '`' {
		j := i + 2
		for j < n && rs[j] != '\n' && !(rs[j] == '`' && j+1 < n && rs[j+1] == '`') {
			j++
		}
		if j > i+2 && j+1 < n && rs[j] == '`' && rs[j+1] == '`' {
			return j + 2, true
		}
	}
	j := i + 1
	for j < n && rs[j] != '`' && rs[j] != '\n' {
		j++
	}
	if j > i+1 && j < n && rs[j] == '`' {
		return j + 1, true
	}
	return 0, false
}

// MaskStructure は見出し・リスト・コードブロック・引用・表・YAML フロントマターの行を
// 空文字に置き換え、インラインコードスパンとリンク URL を空白化したテキストを返す。
// 行数・行番号・行内オフセットは元のテキストと一致する。
//
// インデントコードブロック（4 スペース）は、箇条書きの折り返しや引用の字下げと
// 見分けにくく誤マスクのリスクが高いため対象に含めない。
func MaskStructure(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	var open *Fence
	inFrontMatter := false
	inComment := false
	for idx, line := range lines {
		if idx == 0 && FrontMatterDelimRE.MatchString(line) {
			inFrontMatter = true
			out = append(out, "")
			continue
		}
		if inFrontMatter {
			out = append(out, "")
			if FrontMatterDelimRE.MatchString(line) {
				inFrontMatter = false
			}
			continue
		}
		if f, closeEligible, ok := FenceLine(line); ok {
			switch {
			case open == nil:
				open = &f
			case open.Closes(f, closeEligible):
				open = nil
			}
			out = append(out, "")
			continue
		}
		if open != nil {
			out = append(out, "")
			continue
		}
		line, inComment = maskHTMLCommentsInLine(line, inComment)
		if HeadingRE.MatchString(line) || ListItemRE.MatchString(line) ||
			BlockquoteRE.MatchString(line) || IsTableLine(line) {
			out = append(out, "")
			continue
		}
		out = append(out, blankInlineCodeSpans(line))
	}
	return strings.Join(out, "\n")
}

// HeadingLevelAndText は見出し行から (レベル, 見出しテキスト) を取り出す。
// 末尾の閉じ `#` 列は直前に空白がある場合だけ除去する（「# C#」の # はテキストの一部）。
func HeadingLevelAndText(line string) (int, string) {
	m := headingPartsRE.FindStringSubmatch(line)
	if m == nil {
		return 0, pystr.Strip(line)
	}
	return len(m[1]), pystr.Strip(m[2])
}

// HeadingBody は HeadingRE にマッチした行の、マーカー以降のテキストを返す。
func HeadingBody(line string) (string, bool) {
	loc := HeadingRE.FindStringIndex(line)
	if loc == nil {
		return "", false
	}
	return line[loc[1]:], true
}
