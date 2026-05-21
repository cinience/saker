//go:build integration

package creative_multi_turn_eval

import (
	"context"
	"strings"
	"testing"

	"github.com/saker-ai/saker/eval"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/testutil"
)

type creativeTurn struct {
	Prompt           string
	ExpectedContains []string
	ForbiddenContains []string
	ExpectToolCall   string
}

type creativeMultiTurnCase struct {
	Name      string
	SessionID string
	Turns     []creativeTurn
}

func TestEval_CreativeMultiTurn(t *testing.T) {
	testutil.RequireIntegration(t)

	suite := &eval.EvalSuite{Name: "creative_multi_turn"}
	rt := eval.NewLLMRuntime(t, "",
		eval.WithSystemPrompt("You are a creative production assistant specializing in video and media content creation. Remember all details the user mentions across the conversation. Always respond concisely in Chinese."),
	)

	for _, tc := range creativeMultiTurnCases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			passTurns := 0
			totalChecked := 0

			for i, turn := range tc.Turns {
				resp, err := rt.Run(context.Background(), api.Request{
					Prompt:    turn.Prompt,
					SessionID: tc.SessionID,
				})
				if err != nil {
					t.Fatalf("turn %d: %v", i, err)
				}
				if resp == nil || resp.Result == nil {
					t.Fatalf("turn %d: nil response", i)
				}

				output := strings.ToLower(resp.Result.Output)
				turnPass := true

				// Check expected contains
				if len(turn.ExpectedContains) > 0 {
					totalChecked++
					allFound := false
					for _, expected := range turn.ExpectedContains {
						if strings.Contains(output, strings.ToLower(expected)) {
							allFound = true
							break
						}
					}
					if !allFound {
						turnPass = false
						t.Logf("turn %d: expected one of %v in response, got %q",
							i, turn.ExpectedContains, truncate(resp.Result.Output, 200))
					}
				}

				// Check forbidden contains
				for _, forbidden := range turn.ForbiddenContains {
					if strings.Contains(output, strings.ToLower(forbidden)) {
						turnPass = false
						t.Logf("turn %d: found forbidden %q in response", i, forbidden)
					}
				}

				if turnPass && totalChecked > 0 {
					passTurns++
				} else if len(turn.ExpectedContains) == 0 && len(turn.ForbiddenContains) == 0 {
					// Setup turns without assertions don't count
					continue
				}
			}

			score := 0.0
			if totalChecked > 0 {
				score = float64(passTurns) / float64(totalChecked)
			}
			pass := score >= 0.5

			suite.Add(eval.EvalResult{
				Name:  tc.Name,
				Pass:  pass,
				Score: score,
				Details: map[string]any{
					"pass_turns":    passTurns,
					"total_checked": totalChecked,
					"total_turns":   len(tc.Turns),
				},
			})
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.70 {
			t.Errorf("creative_multi_turn pass rate %.1f%% below 70%% threshold", suite.PassRate()*100)
		}
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
