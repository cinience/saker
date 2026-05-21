package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Baseline stores historical evaluation metrics for regression detection.
type Baseline struct {
	Version     string                  `json:"version"`
	Model       string                  `json:"model"`
	CollectedAt time.Time               `json:"collected_at"`
	Suites      map[string]SuiteBaseline `json:"suites"`
}

// SuiteBaseline holds baseline metrics for a single suite.
type SuiteBaseline struct {
	PassRate  float64 `json:"pass_rate"`
	AvgScore  float64 `json:"avg_score"`
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

// RegressionCheck compares current evaluation results against a baseline.
// Returns alerts sorted by severity (BLOCK first).
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

		// Rule 3: P10 drop > 15% => INFO (requires at least 5 results)
		if len(suite.Results) >= 5 {
			p10 := percentile(suite.Results, 10)
			baselineP10 := bl.AvgScore * 0.7 // approximate P10 as 70% of avg
			if baselineP10 > 0 {
				delta := (baselineP10 - p10) / baselineP10
				if delta > 0.15 {
					alerts = append(alerts, RegressionAlert{
						Level:    RegressionInfo,
						Suite:    suite.Name,
						Metric:   "p10_score",
						Baseline: baselineP10,
						Current:  p10,
						Delta:    -delta,
						Message:  fmt.Sprintf("[INFO] %s P10 score dropped %.1f%%", suite.Name, delta*100),
					})
				}
			}
		}
	}

	return alerts
}

func percentile(results []EvalResult, p int) float64 {
	if len(results) == 0 {
		return 0
	}
	scores := make([]float64, len(results))
	for i, r := range results {
		scores[i] = r.Score
	}
	// Simple sort for percentile
	for i := range scores {
		for j := i + 1; j < len(scores); j++ {
			if scores[j] < scores[i] {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}
	idx := len(scores) * p / 100
	if idx >= len(scores) {
		idx = len(scores) - 1
	}
	return scores[idx]
}
