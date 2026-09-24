package presenter

import (
	"encoding/json"
	"fmt"

	"github.com/ZONO33LHD/japanese-translation-go/usecase/lint"
)

// 前回の `lint --json` 出力のキー名。FindingJSON / WriteLintJSON と同じ場所で持ち、
// 書き出しと読み戻しの形式がずれないようにする。
const (
	keyFindings = "findings"
	keyCategory = "category"
	keyExcerpt  = "excerpt"
)

// DecodeBaseline は --baseline の JSON（前回の `lint --json` 出力）を読み戻す。
// JSON として読めなければ error を返す。
//
// lint は CI ゲートではなく baseline は補助情報なので、形が想定外なら比較を諦めて通常実行に
// フォールバックする（ok=false）。findings の一部の要素だけが不正ならその要素だけ読み飛ばす。
func DecodeBaseline(data []byte) (findings []lint.BaselineFinding, warnings []string, ok bool, err error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		var top any
		if json.Unmarshal(data, &top) != nil {
			return nil, nil, false, fmt.Errorf("decode baseline JSON: %w", err)
		}
		return nil, []string{"--baseline の内容が JSON オブジェクトではありません。baseline比較を無視して通常のlintを実行します。"}, false, nil
	}
	var items []json.RawMessage
	if json.Unmarshal(obj[keyFindings], &items) != nil || items == nil {
		return nil, []string{"--baseline に 'findings' 配列が見つかりません。baseline比較を無視して通常のlintを実行します。"}, false, nil
	}
	skipped := 0
	for _, item := range items {
		var fields map[string]any
		if json.Unmarshal(item, &fields) != nil || fields == nil {
			skipped++
			continue
		}
		cat, catOK := fields[keyCategory].(string)
		exc, excOK := fields[keyExcerpt].(string)
		if !catOK || !excOK {
			skipped++
			continue
		}
		findings = append(findings, lint.BaselineFinding{Category: cat, Excerpt: exc, Raw: item})
	}
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("--baseline の findings 配列内に不正な要素が%d件あったため読み飛ばしました。", skipped))
	}
	return findings, warnings, true, nil
}

// baselineRawJSON は DecodeBaseline が保持した元の JSON を返す。
func baselineRawJSON(f lint.BaselineFinding) json.RawMessage {
	if raw, ok := f.Raw.(json.RawMessage); ok {
		return raw
	}
	b, err := json.Marshal(map[string]string{keyCategory: f.Category, keyExcerpt: f.Excerpt})
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
