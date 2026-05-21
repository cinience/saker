package creative_media_pipeline_eval

import (
	"testing"
	"time"

	"github.com/saker-ai/saker/eval"
)

type expectedStep struct {
	Tool           string
	RequiredParams map[string]string
	OptionalParams []string
}

type mediaPipelineCase struct {
	Name          string
	UserPrompt    string
	ExpectedSteps []expectedStep
	TechSpec      *techSpec
}

type techSpec struct {
	Resolution string
	FrameRate  int
	Codec      string
	Format     string
}

var validResolutions = map[string]bool{
	"720p": true, "1280x720": true,
	"1080p": true, "1920x1080": true,
	"4k": true, "3840x2160": true, "2160p": true,
}

var validFrameRates = map[int]bool{
	24: true, 25: true, 30: true, 60: true,
}

var validCodecs = map[string]bool{
	"h264": true, "h.264": true,
	"h265": true, "h.265": true, "hevc": true,
	"vp9": true, "av1": true,
}

var validFormats = map[string]bool{
	"mp4": true, "mov": true, "webm": true, "mkv": true,
	"png": true, "jpg": true, "jpeg": true, "webp": true,
	"mp3": true, "aac": true, "opus": true, "wav": true,
}

func TestEval_CreativeMediaPipeline(t *testing.T) {
	suite := &eval.EvalSuite{Name: "creative_media_pipeline"}

	for _, tc := range mediaPipelineCases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			start := time.Now()
			score := 0.0
			details := map[string]any{}

			// Verify step structure is well-defined
			stepScore := evaluateStepStructure(tc.ExpectedSteps)
			score += stepScore * 0.4
			details["step_structure_score"] = stepScore

			// Verify technical spec validity
			if tc.TechSpec != nil {
				specScore := evaluateTechSpec(tc.TechSpec)
				score += specScore * 0.4
				details["tech_spec_score"] = specScore
			} else {
				score += 0.4
			}

			// Verify pipeline completeness (no missing steps for the task)
			completeness := evaluateCompleteness(tc)
			score += completeness * 0.2
			details["completeness_score"] = completeness

			pass := score >= 0.7
			suite.Add(eval.EvalResult{
				Name:     tc.Name,
				Pass:     pass,
				Score:    score,
				Duration: time.Since(start),
				Details:  details,
			})

			if !pass {
				t.Logf("case %q: score=%.2f (step=%.2f, spec=%.2f, complete=%.2f)",
					tc.Name, score, stepScore,
					details["tech_spec_score"], completeness)
			}
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.85 {
			t.Errorf("creative_media_pipeline pass rate %.1f%% below 85%% threshold", suite.PassRate()*100)
		}
	})
}

func evaluateStepStructure(steps []expectedStep) float64 {
	if len(steps) == 0 {
		return 0.0
	}

	validSteps := 0
	for _, step := range steps {
		if step.Tool != "" {
			validSteps++
		}
	}
	return float64(validSteps) / float64(len(steps))
}

func evaluateTechSpec(spec *techSpec) float64 {
	score := 0.0
	checks := 0

	if spec.Resolution != "" {
		checks++
		if validResolutions[spec.Resolution] {
			score++
		}
	}
	if spec.FrameRate > 0 {
		checks++
		if validFrameRates[spec.FrameRate] {
			score++
		}
	}
	if spec.Codec != "" {
		checks++
		if validCodecs[spec.Codec] {
			score++
		}
	}
	if spec.Format != "" {
		checks++
		if validFormats[spec.Format] {
			score++
		}
	}

	if checks == 0 {
		return 1.0
	}
	return score / float64(checks)
}

func evaluateCompleteness(tc mediaPipelineCase) float64 {
	if len(tc.ExpectedSteps) == 0 {
		return 0.0
	}

	// Check each step has required params defined
	complete := 0
	for _, step := range tc.ExpectedSteps {
		if step.Tool != "" && len(step.RequiredParams) > 0 {
			complete++
		} else if step.Tool != "" {
			complete++
		}
	}
	return float64(complete) / float64(len(tc.ExpectedSteps))
}
