package presenter

import (
	"fmt"
	"io"

	"github.com/ZONO33LHD/japanese-translation-go/domain/model"
)

// SeverityLabel は重大度の日本語表記。
var SeverityLabel = map[model.Severity]string{
	model.SeverityInfo:     "情報",
	model.SeverityWarn:     "警告",
	model.SeverityCritical: "重大",
}

var statusLabel = map[model.BaselineStatus]string{
	model.StatusNew:        "新規",
	model.StatusPersisting: "継続",
}

// FindingJSON は Finding を元実装の to_dict() と同じキー順の JSON オブジェクトにする。
// status は --baseline 比較時だけ出力する。
func FindingJSON(f model.Finding) Object {
	o := Object{
		{"line", f.Line},
		{keyCategory, f.Category},
		{keyExcerpt, f.Excerpt},
		{"severity", string(f.Severity)},
		{"detail", f.Detail},
		{"related_lines", IntsOrNull(f.RelatedLines)},
	}
	if f.Status != "" {
		o = append(o, KV{"status", string(f.Status)})
	}
	return o
}

// FindingsJSON は Finding の列を JSON 配列用の値にする（空でも null ではなく []）。
func FindingsJSON(fs []model.Finding) []Object {
	out := make([]Object, len(fs))
	for i, f := range fs {
		out[i] = FindingJSON(f)
	}
	return out
}

// WriteFindingHuman は 1 件分の人間可読表示を書き出す。
func WriteFindingHuman(w io.Writer, f model.Finding) {
	label, ok := SeverityLabel[f.Severity]
	if !ok {
		label = string(f.Severity)
	}
	tag := ""
	if f.Status != "" {
		s, ok := statusLabel[f.Status]
		if !ok {
			s = string(f.Status)
		}
		tag = "[" + s + "] "
	}
	fmt.Fprintf(w, "%s[%s] L%d (%s)\n", tag, label, f.Line, sanitizeTerminal(f.Category))
	fmt.Fprintf(w, "    該当箇所: %s\n", sanitizeTerminal(f.Excerpt))
	if f.Detail != "" {
		fmt.Fprintf(w, "    詳細    : %s\n", sanitizeTerminal(f.Detail))
	}
	fmt.Fprintln(w)
}
