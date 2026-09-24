package lint

import "regexp"

// ForbiddenPhrases は禁止語・LLM 常套句のカタログ（拡張前提）。
//
// 2026-07 のコーパス校正で「最後に」「まさに」は削除した。人間側のヒットの大半を
// この 2 語が占めており、AI 特有の手癖ではなく日常語だったため。
var ForbiddenPhrases = []string{
	// 結論の押し付け・まとめ口調
	"と言えるでしょう",
	"と言えるだろう",
	"と言えます",
	"ということになるでしょう",
	"のではないでしょうか",
	"重要なのは",
	"大切なのは",
	"ポイントは",
	"結論から言うと",
	"結論として",
	"いかがでしたか",
	"いかがでしょうか",
	"まとめると",
	"総じて",
	// 過剰な強調・持ち上げ
	"非常に重要",
	"極めて重要",
	"言うまでもなく",
	"言うまでもありません",
	"まさしく",
	// 定型導入・空疎な接続
	"さて、",
	"それでは、",
	"このように",
	"このような中",
	"ここで注目したいのは",
	"見ていきましょう",
	"紹介していきます",
	"解説していきます",
	"深掘りしていきます",
	// 予防線・免責的な言い回し
	"一概には言えません",
	"個人差がありますが",
	"あくまで一例ですが",
	// 正面から系（中身の代わりに姿勢だけを宣言する）
	"正面から扱う",
	"正面から見る",
	"正面から書く",
	"正面から立てる",
	"正面から回収する",
	// 空虚な形容（主張の中身を説明せず強調・網羅感だけ付ける）
	"不可欠",
	"核心的",
	"鍵となる",
	"根本的な",
	"多角的",
	"包括的",
	"総合的",
	// 空虚な動詞・予告口調（何をどう書いたか示さず終わる）
	"掘り下げる",
	"深掘りする",
	"言語化する",
	"について見ていく",
	"を探求する",
}

// ForbiddenPhrasesWeakSignal は人間側でも一定数ヒットするため severity を info に下げる語。
var ForbiddenPhrasesWeakSignal = map[string]bool{
	"重要なのは": true,
	"このように": true,
	"不可欠":   true,
	"ポイントは": true,
	"さて、":   true,
}

// TranslationesePatterns は英語直訳調の表層パターン。
var TranslationesePatterns = []string{
	`することができ(る|ます|た)`,
	`することが可能(です|だ|になる)`,
	`と言えるだろう`,
	`という点で`,
	`という観点(から|で)`,
	`にとって(重要|不可欠)`,
	`を持つ(こと|存在)`,
	`することによって`,
	`であることは間違いない`,
	`に他ならない`,
}

var translationeseREs = compileAll(TranslationesePatterns)

// ParagraphConjunctions は段落頭に来ると構成を接続詞で誤魔化しがちな語。
var ParagraphConjunctions = []string{
	"しかし", "また", "そして", "そのため", "さらに", "つまり",
	"一方", "一方で", "このように", "なぜなら", "したがって", "ただし",
}

// antithesisPatterns は否定→肯定対比の手癖パターン。
var antithesisPatterns = []*regexp.Regexp{
	regexp.MustCompile(`ではなく、?.{0,30}`),
	regexp.MustCompile(`だけでなく.{0,10}も`),
}

// AbstractNounWords は low_specificity で減点する形式名詞・抽象名詞（辞書形で比較）。
// 「こと」「もの」「の」は出現頻度が高すぎて弁別力がないため除外している。
var AbstractNounWords = map[string]bool{
	"側面": true, "観点": true, "重要性": true, "可能性": true, "あり方": true, "存在": true,
	"意味": true, "本質": true, "価値": true, "意義": true, "課題": true, "問題": true,
	"要素": true, "要因": true, "背景": true, "傾向": true, "姿勢": true, "視点": true,
	"概念": true, "特徴": true, "性質": true, "状況": true, "状態": true, "変化": true,
}

// ExampleMarkerWords は段落の具体性を加点する例示マーカー。
var ExampleMarkerWords = []string{
	"たとえば", "例えば", "実際に", "実際には", "具体的には", "具体例として",
	"一例として", "先日", "昨日", "現に", "実例として",
}

var numericQuantityRE = regexp.MustCompile(
	`[0-9０-９]+(年代|年間|世紀|年|月|日|時間|時|分|秒|人|円|%|％|kg|km|cm|mm|g|m|回|件|個|つ|割|倍|台|社|名|冊|本|杯|軒)?`,
)

// 形態素の品詞分類。
var (
	nounEndingPOS     = map[string]bool{"名詞": true}
	trailingSymbolPOS = map[string]bool{"補助記号": true, "空白": true}
	contentWordPOS    = map[string]bool{"名詞": true, "動詞": true, "形容詞": true, "副詞": true}
)

// 「無生物主語+他動詞」判定用の語彙。
// sudachi は「この事実」「そのこと」を 2 形態素に分けるため、単一形態素で成立する語と
// 隣接形態素を連結して比較する語を分けて持つ。
var (
	abstractPronouns       = map[string]bool{"これ": true, "それ": true, "あれ": true, "それら": true}
	abstractPronounPhrases = map[string]bool{"この事実": true, "そのこと": true}
	transitiveSmellVerbs   = map[string]bool{
		"もたらす": true, "示す": true, "意味する": true, "証明する": true, "生み出す": true,
		"反映する": true, "示唆する": true, "物語る": true, "浮き彫りにする": true, "後押しする": true,
	}
)

var (
	inanimateSubjectPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(これ|それ|この事実|そのこと)(は|が).{0,40}(もたらす|示す|意味する|証明する|生み出す|反映する)`),
		regexp.MustCompile(`.{0,20}(こと|事実)(は|が).{0,40}(もたらす|示す|意味する|証明する|生み出す|反映する)`),
	}
	cleftBecauseHead = regexp.MustCompile(`^(それ|これ|この)は.{0,60}(である|だ)$`)
	becauseHead      = regexp.MustCompile(`^(なぜなら|というのも)`)
)

// 文頭がラテン文字主体の製品名・技術用語か（severity の説明を変えるため）。
var latinTechTokenRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9\-_.]*$`)

// 構造層検出器の語彙と正規表現。
var (
	boldSpanRE = regexp.MustCompile(`\*\*[^*\n]+\*\*`)
	// 互いに前方一致する語を含まないので、評価順は結果に影響しない。
	boilerplateHeadingWords = []string{"まとめ", "おわりに", "終わりに", "さいごに", "最後に", "結論", "総括", "conclusion"}
	numberedPhaseRE         = regexp.MustCompile(`(フェーズ|ステップ|段階|ステージ)[` + wsClass + `]*[0-90-9１-９]`)
	emojiSymbolRE           = regexp.MustCompile(`[\x{1F300}-\x{1FAFF}☀-➿⭐✅❌❗❓]`)
)

func compileAll(pats []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(pats))
	for i, p := range pats {
		out[i] = regexp.MustCompile(p)
	}
	return out
}
