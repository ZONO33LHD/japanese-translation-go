package lint

import (
	"regexp"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
	"github.com/ZONO33LHD/japanese-translation-go/domain/service/pystr"
)

// --baseline 差分モードは「lint → 修正 → 再 lint」の収束ループで、前回の --json 出力と
// 比べて resolved / new / persisting に仕分ける。
//
// 同一性キーは (カテゴリ, 空白除去した excerpt の先頭 N 文字)。行番号は修正のたびに
// ずれるので含めない。excerpt は形態素境界や前後の空白の差で完全一致しなくなるため前方一致にする。
const baselineKeyExcerptPrefixLen = 20

// categoryOnlyKeyCategories は excerpt に文書全体の統計量そのものを埋め込んでいるカテゴリ。
// 無関係な編集で数値がわずかに変わるだけで「解消」＋「新規」の偽ペアが出るため、
// カテゴリ名だけをキーにする（1 文書に高々 1 件なので情報は欠けない）。
var categoryOnlyKeyCategories = map[string]bool{
	"low_burstiness":              true,
	"high_length_autocorrelation": true,
	"low_sentence_variance":       true,
	"uniform_paragraph_structure": true,
	"low_lexical_diversity_ttr":   true,
	"low_lexical_diversity_mtld":  true,
}

var whitespaceRE = regexp.MustCompile(`[` + wsClass + `]+`)

type identityKey struct{ category, excerpt string }

func findingIdentityKey(category, excerpt string) identityKey {
	if categoryOnlyKeyCategories[category] {
		return identityKey{category, ""}
	}
	normalized := whitespaceRE.ReplaceAllString(excerpt, "")
	return identityKey{category, pystr.Head(normalized, baselineKeyExcerptPrefixLen)}
}

// BaselineFinding は前回の --json 出力の finding 1 件。
// Raw は解消済みとして報告するとき元の表現をそのまま返すための不透明な値で、usecase は中身を見ない。
type BaselineFinding struct {
	Category string
	Excerpt  string
	Raw      any
}

// BaselineSummary は解消・新規・継続の件数。
type BaselineSummary struct {
	Resolved, New, Persisting int
}

// CompareBaseline は今回の findings に new/persisting を付けた複製と、baseline にしかない
// resolved を返す。同じキーの finding が複数あっても件数分だけ 1 対 1 で対応付ける（多重集合）。
func CompareBaseline(findings []model.Finding, baseline []BaselineFinding) ([]model.Finding, []BaselineFinding, BaselineSummary) {
	buckets := map[identityKey][]BaselineFinding{}
	var order []identityKey
	for _, bf := range baseline {
		k := findingIdentityKey(bf.Category, bf.Excerpt)
		if _, seen := buckets[k]; !seen {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], bf)
	}
	var summary BaselineSummary
	out := make([]model.Finding, len(findings))
	for i, f := range findings {
		k := findingIdentityKey(f.Category, f.Excerpt)
		if b := buckets[k]; len(b) > 0 {
			buckets[k] = b[1:]
			out[i] = f.WithStatus(model.StatusPersisting)
			summary.Persisting++
			continue
		}
		out[i] = f.WithStatus(model.StatusNew)
		summary.New++
	}
	var resolved []BaselineFinding
	for _, k := range order {
		resolved = append(resolved, buckets[k]...)
	}
	summary.Resolved = len(resolved)
	return out, resolved, summary
}
