package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ZONO33LHD/japanese-translation-go/interface/presenter"
	"github.com/ZONO33LHD/japanese-translation-go/usecase/calibrate"
)

const calibrateUsage = "report|sweep --detector NAME|length-analysis [--corpus DIR] [--out DIR] [--dict PATH]"

func runCalibrate(_ context.Context, app *App, args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(app.Stderr, "usage: natural-japanese calibrate %s\n", calibrateUsage)
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
			return ExitOK
		}
		return ExitUsage
	}
	sub := args[0]
	if sub != "report" && sub != "sweep" && sub != "length-analysis" {
		fmt.Fprintf(app.Stderr, "不明なサブコマンドです: %s\n", sub)
		fmt.Fprintf(app.Stderr, "usage: natural-japanese calibrate %s\n", calibrateUsage)
		return ExitUsage
	}

	fs := app.newFlagSet("calibrate "+sub, calibrateUsage)
	corpusDir := fs.String("corpus", "corpus", "校正用コーパスのディレクトリ（human/aozora, human/web, ai, sources.json）")
	outDir := fs.String("out", "", "レポートの出力先（省略時は <corpus>/reports）")
	dict := fs.String("dict", "", "Sudachi システム辞書のパス（省略時は $SUDACHIN_DICT）")
	var detector *string
	if sub == "sweep" {
		detector = fs.String("detector", "", "検出器名（例: low_burstiness）")
	}
	pos, code, ok := app.parseFlags(fs, args[1:])
	if !ok {
		return code
	}
	if len(pos) > 0 {
		fmt.Fprintf(app.Stderr, "余分な引数があります: %s\n", strings.Join(pos, " "))
		return ExitUsage
	}

	var spec calibrate.SweepSpec
	if sub == "sweep" {
		if *detector == "" {
			fmt.Fprintln(app.Stderr, "--detector を指定してください")
			fs.Usage()
			return ExitUsage
		}
		var ok bool
		spec, ok = calibrate.SweepRegistry()[*detector]
		if !ok {
			fmt.Fprintf(app.Stderr, "エラー: 未知の検出器名: %s\n", *detector)
			fmt.Fprintf(app.Stderr, "利用可能: %s\n", strings.Join(calibrate.SweepDetectorNames(), ", "))
			return ExitInputError
		}
	}
	out := *outDir
	if out == "" {
		out = filepath.Join(*corpusDir, "reports")
	}

	files, err := app.NewCorpusLoader(*corpusDir).Load()
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	for _, s := range files.Skipped {
		fmt.Fprintf(app.Stderr, "警告: 読み飛ばしました: %s\n", s)
	}
	if files.Total() == 0 {
		fmt.Fprintf(app.Stderr, "エラー: コーパスに文書がありません（human/aozora, human/web, ai 以下の .md / .txt を探します）: %s\n", *corpusDir)
		return ExitInputError
	}
	corpus := calibrate.GroupCorpus(files)

	tk, closeTk, err := app.OpenTokenizer(*dict)
	if err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	defer func() { _ = closeTk() }()
	runner := calibrate.NewRunner(tk)

	var name string
	var md string
	var jsonValue any
	switch sub {
	case "report":
		prepared := map[calibrate.Group][]calibrate.Prepared{}
		for _, g := range calibrate.Groups {
			ps, err := runner.PrepareWithFindings(corpus[g])
			if err != nil {
				fmt.Fprintln(app.Stderr, err)
				return ExitInputError
			}
			prepared[g] = ps
		}
		r := calibrate.BuildReport(prepared)
		name, md, jsonValue = "report", presenter.CalibrateReportMarkdown(r), presenter.CalibrateReportJSON(r)
	case "sweep":
		human, err := runner.Prepare(corpus.Humans())
		if err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		ai, err := runner.Prepare(corpus[calibrate.AI])
		if err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		r, err := calibrate.Sweep(tk, *detector, spec, human, ai)
		if err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		name, md, jsonValue = "sweep_"+*detector, presenter.CalibrateSweepMarkdown(r), presenter.CalibrateSweepJSON(r)
	case "length-analysis":
		human, err := runner.PrepareWithFindings(corpus.Humans())
		if err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		ai, err := runner.PrepareWithFindings(corpus[calibrate.AI])
		if err != nil {
			fmt.Fprintln(app.Stderr, err)
			return ExitInputError
		}
		r := calibrate.AnalyzeLength(human, ai)
		name, md, jsonValue = "length_analysis", presenter.CalibrateLengthMarkdown(r), presenter.CalibrateLengthJSON(r)
	}
	if err := app.writeCalibrateOutputs(out, name, md, jsonValue); err != nil {
		fmt.Fprintln(app.Stderr, err)
		return ExitInputError
	}
	return ExitOK
}

// writeCalibrateOutputs は Markdown と JSON を out に保存し、保存先と Markdown 本文を stdout に出す。
func (app *App) writeCalibrateOutputs(out, name, md string, jsonValue any) error {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return fmt.Errorf("create report directory %s: %w", out, err)
	}
	var buf bytes.Buffer
	if err := presenter.WriteJSON(&buf, jsonValue); err != nil {
		return fmt.Errorf("encode %s.json: %w", name, err)
	}
	mdPath := filepath.Join(out, name+".md")
	jsonPath := filepath.Join(out, name+".json")
	// 元実装は末尾改行なしで JSON を書いていたため、WriteJSON の改行を落として揃える。
	if err := os.WriteFile(jsonPath, bytes.TrimRight(buf.Bytes(), "\n"), 0o640); err != nil {
		return fmt.Errorf("write %s: %w", jsonPath, err)
	}
	if err := os.WriteFile(mdPath, []byte(md), 0o640); err != nil {
		return fmt.Errorf("write %s: %w", mdPath, err)
	}
	fmt.Fprintf(app.Stdout, "書き出し: %s\n", mdPath)
	fmt.Fprintf(app.Stdout, "書き出し: %s\n", jsonPath)
	fmt.Fprintln(app.Stdout)
	fmt.Fprint(app.Stdout, md)
	return nil
}
