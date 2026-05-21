//go:build integration

package creative_media_quality_eval

import (
	"context"
	_ "embed"
	"testing"

	"github.com/saker-ai/saker/eval"
	"github.com/saker-ai/saker/pkg/api"
	"github.com/saker-ai/saker/pkg/testutil"
)

//go:embed judge_prompt.txt
var judgePromptTemplate string

type mediaQualityCase struct {
	Name       string
	Prompt     string
	MinScore   float64
	Dimensions []string
}

func cases() []mediaQualityCase {
	return []mediaQualityCase{
		{
			Name:       "product_video_plan",
			Prompt:     "为一款智能手表设计一个 30 秒的产品宣传视频方案，包括镜头列表、转场、配乐建议",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "aesthetic_coherence", "constraint"},
		},
		{
			Name:       "music_video_concept",
			Prompt:     "设计一个电子音乐 MV 的视觉概念，要求赛博朋克风格，包含 5 个关键场景描述",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "aesthetic_coherence"},
		},
		{
			Name:       "podcast_cover_design",
			Prompt:     "为一档科技播客设计封面图方案，尺寸 3000x3000，需要在小尺寸下也清晰可辨",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "constraint"},
		},
		{
			Name:       "social_media_video_series",
			Prompt:     "规划一组 5 条抖音竖屏短视频的拍摄方案，主题是咖啡制作教程，每条 15-30 秒",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "constraint"},
		},
		{
			Name:       "corporate_intro_video",
			Prompt:     "为一家 AI 创业公司设计 60 秒企业介绍视频方案，要求现代简约风，包含数据可视化动画",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "aesthetic_coherence"},
		},
		{
			Name:       "live_stream_setup",
			Prompt:     "设计一个双机位直播间的画面布局和切换方案，用于产品发布会",
			MinScore:   0.5,
			Dimensions: []string{"specificity", "feasibility"},
		},
		{
			Name:       "animation_storyboard",
			Prompt:     "为一个 2D 动画短片设计分镜脚本，主题是一只猫的冒险故事，3 分钟",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "aesthetic_coherence"},
		},
		{
			Name:       "photo_series_concept",
			Prompt:     "设计一组 10 张产品摄影方案，白底棚拍，展示不同角度和使用场景",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "constraint"},
		},
		{
			Name:       "video_color_grading",
			Prompt:     "为一部城市纪录片设计调色方案，参考电影《银翼杀手2049》的色彩风格",
			MinScore:   0.5,
			Dimensions: []string{"specificity", "aesthetic_coherence"},
		},
		{
			Name:       "multi_platform_export",
			Prompt:     "设计一套视频多平台分发方案：YouTube(16:9 4K)、Instagram Reels(9:16 1080p)、Twitter(1:1 720p)",
			MinScore:   0.6,
			Dimensions: []string{"specificity", "feasibility", "constraint"},
		},
	}
}

func TestEval_CreativeMediaQuality(t *testing.T) {
	testutil.RequireIntegration(t)

	suite := &eval.EvalSuite{Name: "creative_media_quality"}
	rt := eval.NewLLMRuntime(t, "",
		eval.WithSystemPrompt("You are a professional media production expert. When given a media production task, provide detailed, technically accurate plans. Always respond in Chinese."),
	)
	judgeMdl := eval.NewLLMModel(t, "")
	judge := &eval.LLMJudge{Model: judgeMdl, MaxRetries: 2}

	for _, tc := range cases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			resp, err := rt.Run(context.Background(), api.Request{
				Prompt:    tc.Prompt,
				SessionID: "media-quality-" + tc.Name,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if resp == nil || resp.Result == nil || resp.Result.Output == "" {
				t.Fatalf("empty response")
			}

			result, err := judge.Judge(context.Background(), judgePromptTemplate, tc.Prompt, resp.Result.Output)
			if err != nil {
				t.Logf("judge error (non-fatal): %v", err)
				suite.Add(eval.EvalResult{
					Name:  tc.Name,
					Pass:  false,
					Score: 0,
					Got:   "judge error: " + err.Error(),
				})
				return
			}

			pass := result.Overall >= tc.MinScore
			suite.Add(eval.EvalResult{
				Name:  tc.Name,
				Pass:  pass,
				Score: result.Overall,
				Details: map[string]any{
					"dimensions": result.Dimensions,
					"reasoning":  result.Reasoning,
				},
			})

			if !pass {
				t.Logf("case %q: overall=%.2f (min=%.2f), dims=%v",
					tc.Name, result.Overall, tc.MinScore, result.Dimensions)
			}
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.60 {
			t.Errorf("creative_media_quality pass rate %.1f%% below 60%% threshold", suite.PassRate()*100)
		}
	})
}
