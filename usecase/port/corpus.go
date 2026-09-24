package port

// CorpusDocument は校正用コーパスの 1 文書。
type CorpusDocument struct {
	Path string
	Text string
}

// CorpusFiles は校正用コーパスの生の読み込み結果。ジャンル分けはユースケース側で行う。
type CorpusFiles struct {
	Aozora []CorpusDocument
	Web    []CorpusDocument
	AI     []CorpusDocument
	// GenreByID は sources.json の id → genre。ファイルが無い・壊れている場合は空。
	GenreByID map[string]string
	// Skipped は読み飛ばしたファイルと理由（利用者に警告するため）。
	Skipped []string
}

// Total は読み込めた文書の総数。
func (c CorpusFiles) Total() int { return len(c.Aozora) + len(c.Web) + len(c.AI) }

// CorpusLoader は校正用コーパスを読み込む。
// サブディレクトリが欠けていても読めた分だけ返し（部分的なコーパスでも動かすため）、
// 読み飛ばしたファイルは Skipped に記録する。コーパスのディレクトリ自体が無ければ error。
type CorpusLoader interface {
	Load() (CorpusFiles, error)
}
