// Package model は検査層の中核となるエンティティと値オブジェクトを定義する。
// 外側の層（usecase / interface / infrastructure）には一切依存しない。
package model

import "slices"

// Severity は指摘の重大度。
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

// BaselineStatus は --baseline 比較時の新規/継続の区別。比較しない通常実行では空。
type BaselineStatus string

const (
	StatusNew        BaselineStatus = "new"
	StatusPersisting BaselineStatus = "persisting"
)

// Finding は検出器 1 件分の指摘。
type Finding struct {
	Line     int
	Category string
	Excerpt  string
	Severity Severity
	Detail   string
	// RelatedLines は文書全体集計型の検出器で、同じ集計に基づく他の該当行を列挙する。
	// 単発検出では nil のまま。
	RelatedLines []int
	Status       BaselineStatus
}

// NewFinding は RelatedLines を重複除去・昇順に正規化した Finding を返す。
func NewFinding(line int, category, excerpt string, severity Severity, detail string, related []int) Finding {
	return Finding{
		Line:         line,
		Category:     category,
		Excerpt:      excerpt,
		Severity:     severity,
		Detail:       detail,
		RelatedLines: normalizeLines(related),
	}
}

// WithStatus は Status を差し替えた複製を返す。
func (f Finding) WithStatus(s BaselineStatus) Finding {
	f.Status = s
	return f
}

func normalizeLines(lines []int) []int {
	if lines == nil {
		return nil
	}
	out := slices.Clone(lines)
	slices.Sort(out)
	return slices.Compact(out)
}

// SortFindingsByLine は行番号で安定ソートした新しいスライスを返す。
func SortFindingsByLine(fs []Finding) []Finding {
	out := slices.Clone(fs)
	slices.SortStableFunc(out, func(a, b Finding) int { return a.Line - b.Line })
	return out
}
