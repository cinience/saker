package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/saker-ai/saker/eval"
)

func timeNow() time.Time { return time.Now().UTC() }

func runCompare(baselinePath, candidatePath string) error {
	baseline, err := eval.LoadBaseline(baselinePath)
	if err != nil {
		return fmt.Errorf("load baseline: %w", err)
	}

	candidate, err := eval.LoadBaseline(candidatePath)
	if err != nil {
		return fmt.Errorf("load candidate: %w", err)
	}

	fmt.Println("=== A/B Comparison ===")
	fmt.Printf("  Baseline:  %s (model=%s, %s)\n", baselinePath, baseline.Model, baseline.CollectedAt.Format("2006-01-02"))
	fmt.Printf("  Candidate: %s (model=%s, %s)\n", candidatePath, candidate.Model, candidate.CollectedAt.Format("2006-01-02"))
	fmt.Println()

	// Collect all suite names
	suiteNames := map[string]bool{}
	for k := range baseline.Suites {
		suiteNames[k] = true
	}
	for k := range candidate.Suites {
		suiteNames[k] = true
	}

	sorted := make([]string, 0, len(suiteNames))
	for k := range suiteNames {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	hasRegression := false
	for _, name := range sorted {
		bl, blOK := baseline.Suites[name]
		cd, cdOK := candidate.Suites[name]

		if !blOK {
			fmt.Printf("  %-30s -- (new, avg=%.2f)\n", name, cd.AvgScore)
			continue
		}
		if !cdOK {
			fmt.Printf("  %-30s -- (removed)\n", name)
			continue
		}

		delta := cd.AvgScore - bl.AvgScore
		pct := 0.0
		if bl.AvgScore > 0 {
			pct = delta / bl.AvgScore * 100
		}

		status := "STABLE"
		if pct > 3.0 {
			status = "IMPROVED"
		} else if pct < -3.0 {
			status = "REGRESSED"
			hasRegression = true
		}

		fmt.Printf("  %-30s %.2f -> %.2f (%+.1f%%) [%s]\n",
			name, bl.AvgScore, cd.AvgScore, pct, status)
	}

	fmt.Println()
	if hasRegression {
		fmt.Println("Result: REGRESSIONS DETECTED")
		os.Exit(1)
	}
	fmt.Println("Result: No regressions")
	return nil
}

func runTrend(reportsDir string) error {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		return fmt.Errorf("read reports dir: %w", err)
	}

	var baselines []*eval.Baseline
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		bl, err := eval.LoadBaseline(filepath.Join(reportsDir, e.Name()))
		if err != nil {
			continue
		}
		baselines = append(baselines, bl)
	}

	if len(baselines) == 0 {
		fmt.Println("No baseline files found in", reportsDir)
		return nil
	}

	// Sort by date
	sort.Slice(baselines, func(i, j int) bool {
		return baselines[i].CollectedAt.Before(baselines[j].CollectedAt)
	})

	// Collect all suite names
	suiteNames := map[string]bool{}
	for _, bl := range baselines {
		for k := range bl.Suites {
			suiteNames[k] = true
		}
	}
	sorted := make([]string, 0, len(suiteNames))
	for k := range suiteNames {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	// Print header
	fmt.Printf("| %-10s |", "Date")
	for _, name := range sorted {
		short := name
		if len(short) > 12 {
			short = short[:12]
		}
		fmt.Printf(" %-12s |", short)
	}
	fmt.Println()

	// Separator
	fmt.Printf("|%-12s|", strings.Repeat("-", 12))
	for range sorted {
		fmt.Printf("%-14s|", strings.Repeat("-", 14))
	}
	fmt.Println()

	// Data rows
	for _, bl := range baselines {
		fmt.Printf("| %-10s |", bl.CollectedAt.Format("2006-01-02"))
		for _, name := range sorted {
			if sb, ok := bl.Suites[name]; ok {
				fmt.Printf(" %-12.2f |", sb.AvgScore)
			} else {
				fmt.Printf(" %-12s |", "--")
			}
		}
		fmt.Println()
	}

	return nil
}

// exportReport writes current run results as a baseline JSON for trend tracking.
func exportReport(results []suiteResult, outputPath, modelName string) error {
	bl := &eval.Baseline{
		Version:     "1",
		Model:       modelName,
		CollectedAt: timeNow(),
		Suites:      make(map[string]eval.SuiteBaseline),
	}

	for _, r := range results {
		score := 0.0
		if r.Pass {
			score = 1.0
		}
		bl.Suites[r.Suite] = eval.SuiteBaseline{
			PassRate:  score,
			AvgScore:  score,
			CaseCount: 1,
		}
	}

	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}
