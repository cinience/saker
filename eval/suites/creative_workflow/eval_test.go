//go:build integration

package creative_workflow_eval

import (
	"context"
	"testing"
	"time"

	"github.com/saker-ai/saker/eval"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/testutil"
)

type workflowCase struct {
	Name         string
	Prompt       string
	OptimalSteps int
	MaxSteps     int
}

func cases() []workflowCase {
	return []workflowCase{
		{
			Name:         "simple_video_analysis",
			Prompt:       "分析 demo.mp4 的画面内容，告诉我主要展示了什么产品",
			OptimalSteps: 2,
			MaxSteps:     5,
		},
		{
			Name:         "create_storyboard",
			Prompt:       "为一个 30 秒的手机广告创建故事板，包含 6 个关键帧的描述",
			OptimalSteps: 1,
			MaxSteps:     4,
		},
		{
			Name:         "media_search_compile",
			Prompt:       "从素材库中找出所有适合做片头的城市航拍素材，列出文件名和时长",
			OptimalSteps: 2,
			MaxSteps:     5,
		},
		{
			Name:         "multi_platform_adapt",
			Prompt:       "将一个横版 16:9 的视频方案改编为抖音竖版 9:16 的方案，保留核心内容",
			OptimalSteps: 1,
			MaxSteps:     3,
		},
		{
			Name:         "creative_brief_to_plan",
			Prompt:       "根据以下创意简报生成完整制作计划：产品是无线耳机，目标是展示降噪功能，时长 45 秒，风格科技感",
			OptimalSteps: 1,
			MaxSteps:     4,
		},
		{
			Name:         "batch_thumbnail_review",
			Prompt:       "检查视频的 5 个候选封面图，选出构图最佳的一张并说明原因",
			OptimalSteps: 3,
			MaxSteps:     8,
		},
		{
			Name:         "script_to_shotlist",
			Prompt:       "把以下脚本转换为镜头列表：场景一，咖啡厅内景，主角坐在窗边看书，阳光洒在书页上。服务员端来咖啡，两人对视微笑。",
			OptimalSteps: 1,
			MaxSteps:     3,
		},
		{
			Name:         "full_production_workflow",
			Prompt:       "完整规划一个产品开箱视频的制作流程：从脚本构思到最终发布，列出所有步骤和所需工具",
			OptimalSteps: 2,
			MaxSteps:     6,
		},
	}
}

func TestEval_CreativeWorkflow(t *testing.T) {
	testutil.RequireIntegration(t)

	suite := &eval.EvalSuite{Name: "creative_workflow"}
	rt := eval.NewLLMRuntime(t, "",
		eval.WithSystemPrompt("You are a creative production assistant. Complete tasks efficiently using the minimum necessary steps. Always respond in Chinese."),
	)

	for _, tc := range cases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			start := time.Now()

			resp, err := rt.Run(context.Background(), api.Request{
				Prompt:    tc.Prompt,
				SessionID: "workflow-" + tc.Name,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}

			wallTime := time.Since(start)
			completed := resp != nil && resp.Result != nil && resp.Result.Output != ""

			// Count tool calls as steps
			actualSteps := 0
			if resp != nil && resp.Result != nil {
				actualSteps = len(resp.Result.ToolCalls)
				if actualSteps == 0 {
					actualSteps = 1 // text response counts as 1 step
				}
			}

			// Calculate efficiency score
			completionScore := 0.0
			if completed {
				completionScore = 1.0
			}

			redundancy := 0.0
			if tc.OptimalSteps > 0 && actualSteps > tc.OptimalSteps {
				redundancy = float64(actualSteps-tc.OptimalSteps) / float64(tc.OptimalSteps)
			}
			if redundancy > 1.0 {
				redundancy = 1.0
			}

			// Simplified efficiency formula (no baseline token/time data for first run)
			efficiencyScore := completionScore*0.4 + (1.0-redundancy)*0.3 + 0.3 // 0.3 placeholder for token+time

			pass := completed && actualSteps <= tc.MaxSteps
			score := efficiencyScore
			if !completed {
				score = 0.0
			}

			suite.Add(eval.EvalResult{
				Name:     tc.Name,
				Pass:     pass,
				Score:    score,
				Duration: wallTime,
				Details: map[string]any{
					"completed":     completed,
					"actual_steps":  actualSteps,
					"optimal_steps": tc.OptimalSteps,
					"max_steps":     tc.MaxSteps,
					"redundancy":    redundancy,
					"wall_time_ms":  wallTime.Milliseconds(),
				},
			})

			if !pass {
				t.Logf("case %q: completed=%v, steps=%d (optimal=%d, max=%d)",
					tc.Name, completed, actualSteps, tc.OptimalSteps, tc.MaxSteps)
			}
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.60 {
			t.Errorf("creative_workflow pass rate %.1f%% below 60%% threshold", suite.PassRate()*100)
		}
	})
}
