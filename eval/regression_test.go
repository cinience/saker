package eval

import (
	"testing"
)

func TestRegressionCheck_NoRegression(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "test_suite", Results: []EvalResult{
				{Pass: true, Score: 0.9},
				{Pass: true, Score: 0.8},
				{Pass: true, Score: 0.85},
				{Pass: true, Score: 0.9},
				{Pass: true, Score: 0.88},
			}},
		},
	}
	baseline := &Baseline{
		Suites: map[string]SuiteBaseline{
			"test_suite": {PassRate: 0.90, AvgScore: 0.85, P10Score: 0.75, CaseCount: 5},
		},
	}

	alerts := RegressionCheck(report, baseline)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts, got %d: %+v", len(alerts), alerts)
	}
}

func TestRegressionCheck_BlockOnPassRateDrop(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "test_suite", Results: []EvalResult{
				{Pass: true, Score: 0.9},
				{Pass: false, Score: 0.3},
				{Pass: false, Score: 0.2},
				{Pass: false, Score: 0.4},
				{Pass: true, Score: 0.8},
			}},
		},
	}
	baseline := &Baseline{
		Suites: map[string]SuiteBaseline{
			"test_suite": {PassRate: 0.90, AvgScore: 0.85, P10Score: 0.75, CaseCount: 5},
		},
	}

	alerts := RegressionCheck(report, baseline)
	hasBlock := false
	for _, a := range alerts {
		if a.Level == RegressionBlock && a.Metric == "pass_rate" {
			hasBlock = true
		}
	}
	if !hasBlock {
		t.Errorf("expected BLOCK alert for pass_rate drop, got: %+v", alerts)
	}
}

func TestRegressionCheck_WarnOnAvgScoreDrop(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "test_suite", Results: []EvalResult{
				{Pass: true, Score: 0.7},
				{Pass: true, Score: 0.75},
				{Pass: true, Score: 0.72},
			}},
		},
	}
	baseline := &Baseline{
		Suites: map[string]SuiteBaseline{
			"test_suite": {PassRate: 1.0, AvgScore: 0.90, P10Score: 0.80, CaseCount: 3},
		},
	}

	alerts := RegressionCheck(report, baseline)
	hasWarn := false
	for _, a := range alerts {
		if a.Level == RegressionWarn && a.Metric == "avg_score" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected WARN alert for avg_score drop, got: %+v", alerts)
	}
}

func TestRegressionCheck_InfoOnP10Drop(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "test_suite", Results: []EvalResult{
				{Pass: true, Score: 0.9},
				{Pass: true, Score: 0.85},
				{Pass: true, Score: 0.8},
				{Pass: true, Score: 0.4},
				{Pass: true, Score: 0.3},
			}},
		},
	}
	baseline := &Baseline{
		Suites: map[string]SuiteBaseline{
			"test_suite": {PassRate: 1.0, AvgScore: 0.85, P10Score: 0.75, CaseCount: 5},
		},
	}

	alerts := RegressionCheck(report, baseline)
	hasInfo := false
	for _, a := range alerts {
		if a.Level == RegressionInfo && a.Metric == "p10_score" {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Errorf("expected INFO alert for P10 drop, got: %+v", alerts)
	}
}

func TestRegressionCheck_UnknownSuiteIgnored(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "new_suite", Results: []EvalResult{{Pass: false, Score: 0.1}}},
		},
	}
	baseline := &Baseline{
		Suites: map[string]SuiteBaseline{
			"other_suite": {PassRate: 0.9, AvgScore: 0.8, CaseCount: 5},
		},
	}

	alerts := RegressionCheck(report, baseline)
	if len(alerts) != 0 {
		t.Errorf("expected no alerts for unknown suite, got %d", len(alerts))
	}
}

func TestComputeP10(t *testing.T) {
	results := []EvalResult{
		{Score: 0.1},
		{Score: 0.3},
		{Score: 0.5},
		{Score: 0.7},
		{Score: 0.8},
		{Score: 0.85},
		{Score: 0.9},
		{Score: 0.92},
		{Score: 0.95},
		{Score: 1.0},
	}
	p10 := computeP10(results)
	// 10th percentile of 10 items => index 1 => 0.3
	if p10 != 0.3 {
		t.Errorf("computeP10: want 0.3, got %f", p10)
	}
}

func TestCollectBaseline(t *testing.T) {
	report := &EvalReport{
		Suites: []EvalSuite{
			{Name: "suite_a", Results: []EvalResult{
				{Pass: true, Score: 0.9},
				{Pass: true, Score: 0.8},
				{Pass: false, Score: 0.4},
			}},
		},
	}

	bl := CollectBaseline(report, "test-model")
	if bl.Model != "test-model" {
		t.Errorf("model: want test-model, got %s", bl.Model)
	}
	sb, ok := bl.Suites["suite_a"]
	if !ok {
		t.Fatal("suite_a not found in baseline")
	}
	if sb.CaseCount != 3 {
		t.Errorf("case_count: want 3, got %d", sb.CaseCount)
	}
	// pass_rate = 2/3 ≈ 0.667
	if sb.PassRate < 0.66 || sb.PassRate > 0.67 {
		t.Errorf("pass_rate: want ~0.667, got %f", sb.PassRate)
	}
}
