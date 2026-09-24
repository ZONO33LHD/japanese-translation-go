package calibrate

// LengthBin は文書長ビン。Hi<0 は上限なし。
type LengthBin struct {
	Label  string
	Lo, Hi int
}

// LengthBins は length-analysis の文書長ビン。
var LengthBins = []LengthBin{
	{"~1000字", 0, 1000},
	{"~2000字", 1000, 2000},
	{"~4000字", 2000, 4000},
	{"4000字~", 4000, -1},
}

// BinForLength は文字数が属するビンのラベルを返す。
func BinForLength(n int) string {
	for _, b := range LengthBins {
		if b.Hi < 0 {
			if n >= b.Lo {
				return b.Label
			}
		} else if b.Lo <= n && n < b.Hi {
			return b.Label
		}
	}
	return LengthBins[len(LengthBins)-1].Label
}

// BinCell はビン × カテゴリの発火件数と標本数。
type BinCell struct {
	HumanFired, HumanTotal, AIFired, AITotal int
}

// LengthAnalysis は統計指標系検出器の文書長別弁別力。
type LengthAnalysis struct {
	NHuman int
	NAI    int
	// Bins[label][category]
	Bins map[string]map[string]BinCell
	// MinEffectiveLengthBin[category] は判定不能なら空文字。
	MinEffectiveLengthBin map[string]string
}

// AnalyzeLength は文書長ビンごとに人間/AI の発火率を集計し、
// AI 発火率が人間を上回り始める最小のビン（弁別力が正になる最小の文書長）を推定する。
func AnalyzeLength(human, ai []Prepared) LengthAnalysis {
	res := LengthAnalysis{NHuman: len(human), NAI: len(ai), Bins: map[string]map[string]BinCell{}, MinEffectiveLengthBin: map[string]string{}}
	for _, b := range LengthBins {
		row := map[string]BinCell{}
		for _, cat := range StatisticalCategories {
			row[cat] = BinCell{}
		}
		res.Bins[b.Label] = row
	}
	for _, p := range human {
		row := res.Bins[BinForLength(p.Doc.CharCount)]
		for _, cat := range StatisticalCategories {
			c := row[cat]
			c.HumanTotal++
			if fired(p.Findings, cat) {
				c.HumanFired++
			}
			row[cat] = c
		}
	}
	for _, p := range ai {
		row := res.Bins[BinForLength(p.Doc.CharCount)]
		for _, cat := range StatisticalCategories {
			c := row[cat]
			c.AITotal++
			if fired(p.Findings, cat) {
				c.AIFired++
			}
			row[cat] = c
		}
	}
	for _, cat := range StatisticalCategories {
		res.MinEffectiveLengthBin[cat] = ""
		for _, b := range LengthBins {
			c := res.Bins[b.Label][cat]
			if c.HumanTotal == 0 || c.AITotal == 0 {
				continue
			}
			if float64(c.AIFired)/float64(c.AITotal) > float64(c.HumanFired)/float64(c.HumanTotal) {
				res.MinEffectiveLengthBin[cat] = b.Label
				break
			}
		}
	}
	return res
}
