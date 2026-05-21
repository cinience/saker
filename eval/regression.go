package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Baseline stores historical evaluation metrics for regression detection.
type Baseline struct {
	Version     string                   `json:"version"`
	Model       string                   `json:"model"`
	CollectedAt time.Time                `json:"collected_at"`
	Suites      map[string]SuiteBaseline `json:"suites"`
}

// SuiteBaseline holds baseline metrics for a single suite.
type SuiteBaseline struct {
	PassRate  float64 `json:"pass_rate"`
	AvgScore  float64 `json:"avg_score"`
	P10Score  float64 `json:"p10_score"`
	CaseCount int     `json:"case_count"`
}

// RegressionLevel indicates the severity of a regression.
type RegressionLevel string

const (
	RegressionBlock RegressionLevel = "BLOCK"
	RegressionWarn  RegressionLevel = "WARN"
	RegressionInfo  RegressionLevel = "INFO"
)

// RegressionAlert describes a detected regression.
type RegressionAlert struct {
	Level    RegressionLevel `json:"level"`
	Suite    string          `json:"suite"`
	Metric   string          `json:"metric"`
	Baseline float64         `json:"baseline"`
	Current  float64         `json:"current"`
	Delta    float64         `json:"delta"`
	Message  string          `json:"message"`
}

// LoadBaseline reads a baseline JSON file.
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load baseline: %w", err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	return &b, nil
}

// SaveBaseline writes a baseline JSON file.
func SaveBaseline(path string, b *Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// CollectBaseline generates a Baseline from an EvalReport.
func CollectBaseline(report *EvalReport, modelName string) *Baseline {
	b := &Baseline{
		Version:     "1",
		Model:       modelName,
		CollectedAt: time.Now().UTC(),
		Suites:      make(map[string]SuiteBaseline),
	}
	for i := range report.Suites {
		suite := &report.Suites[i]
		b.Suites[suite.Name] = SuiteBaseline{
			PassRate:  suite.PassRate(),
			AvgScore:  suite.AvgScore(),
			P10Score:  computeP10(suite.Results),
			CaseCount: len(suite.Results),
		}
	}
	return b
}

// RegressionCheck compares current evaluation results against a baseline.
func RegressionCheck(report *EvalReport, baseline *Baseline) []RegressionAlert {
	var alerts []RegressionAlert

	for i := range report.Suites {
		suite := &report.Suites[i]
		bl, ok := baseline.Suites[suite.Name]
		if !ok {
			continue
		}

		passRate := suite.PassRate()
		avgScore := suite.AvgScore()

		// Rule 1: pass_rate drop > 10% => BLOCK
		if bl.PassRate > 0 {
			delta := (bl.PassRate - passRate) / bl.PassRate
			if delta > 0.10 {
				alerts = append(alerts, RegressionAlert{
					Level:    RegressionBlock,
					Suite:    suite.Name,
					Metric:   "pass_rate",
					Baseline: bl.PassRate,
					Current:  passRate,
					Delta:    -delta,
					Message:  fmt.Sprintf("[BLOCK] %s pass_rate dropped %.1f%% (%.2f -> %.2f)", suite.Name, delta*100, bl.PassRate, passRate),
				})
			}
		}

		// Rule 2: avg_score drop > 5% => WARN
		if bl.AvgScore > 0 {
			delta := (bl.AvgScore - avgScore) / bl.AvgScore
			if delta > 0.05 {
				alerts = append(alerts, RegressionAlert{
					Level:    RegressionWarn,
					Suite:    suite.Name,
					Metric:   "avg_score",
					Baseline: bl.AvgScore,
					Current:  avgScore,
					Delta:    -delta,
					Message:  fmt.Sprintf("[WARN] %s avg_score dropped %.1f%% (%.2f -> %.2f)", suite.Name, delta*100, bl.AvgScore, avgScore),
				})
			}
		}

		// Rule 3: P10 drop > 15% => INFO
		if bl.P10Score > 0 && len(suite.Results) >= 5 {
			p10 := computeP10(suite.Results)
			delta := (bl.P10Score - p10) / bl.P10Score
			if delta > 0.15 {
				alerts = append(alerts, RegressionAlert{
					Level:    RegressionInfo,
					Suite:    suite.Name,
					Metric:   "p10_score",
					Baseline: bl.P10Score,
					Current:  p10,
					Delta:    -delta,
					Message:  fmt.Sprintf("[INFO] %s P10 score dropped %.1f%% (%.2f -> %.2f)", suite.Name, delta*100, bl.P10Score, p10),
				})
			}
		}
	}

	return alerts
}

func computeP10(results []EvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	scores := make([]float64, len(results))
	for i, r := range results {
		scores[i] = r.Score
	}
	sort.Float64s(scores)
	idx := len(scores) * 10 / 100
	if idx >= len(scores) {
		idx = len(scores) - 1
	}
	return scores[idx]
}
