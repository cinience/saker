//go:build integration

package creative_writing_eval

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

type creativeWritingCase struct {
	Name       string
	Category   string // script, story, divergent
	Prompt     string
	MinScore   float64
	Constraints []string
}

func cases() []creativeWritingCase {
	return []creativeWritingCase{
		// === 脚本编写 (5 cases) ===
		{
			Name:     "short_film_script",
			Category: "script",
			Prompt:   "写一个 3 分钟的短片脚本，主题是'城市中的孤独'，包含场景描述、对话和镜头指示",
			MinScore: 0.6,
			Constraints: []string{"场景", "对话", "镜头"},
		},
		{
			Name:     "commercial_script_30s",
			Category: "script",
			Prompt:   "为一款运动饮料写一个 30 秒电视广告脚本，目标受众是 18-25 岁年轻人，要有活力感",
			MinScore: 0.6,
			Constraints: []string{"30秒", "年轻"},
		},
		{
			Name:     "documentary_narration",
			Category: "script",
			Prompt:   "为一部关于海洋保护的纪录片写 2 分钟的开场旁白，风格庄重而不沉重",
			MinScore: 0.6,
			Constraints: []string{"海洋", "旁白"},
		},
		{
			Name:     "podcast_intro_script",
			Category: "script",
			Prompt:   "为一档科技播客写开场白脚本，15 秒，需要包含节目名称'未来电波'和本期主题'AI 创作'",
			MinScore: 0.6,
			Constraints: []string{"未来电波", "AI"},
		},
		{
			Name:     "interview_questions",
			Category: "script",
			Prompt:   "为一位独立游戏开发者的采访设计 8 个问题，从个人经历到行业观点，要有递进关系",
			MinScore: 0.5,
			Constraints: []string{"游戏", "问题"},
		},

		// === 故事生成 (5 cases) ===
		{
			Name:     "micro_fiction",
			Category: "story",
			Prompt:   "写一个 500 字以内的微小说，主题是'时间旅行者的遗憾'，要有反转结局",
			MinScore: 0.6,
			Constraints: []string{"时间", "反转"},
		},
		{
			Name:     "brand_story",
			Category: "story",
			Prompt:   "为一家手工咖啡品牌写一个品牌故事，突出'匠心'和'从产地到杯中'的理念，200字左右",
			MinScore: 0.6,
			Constraints: []string{"咖啡", "匠心"},
		},
		{
			Name:     "children_story_outline",
			Category: "story",
			Prompt:   "写一个适合 5-8 岁儿童的绘本故事大纲，主角是一只会飞的小企鹅，要有教育意义",
			MinScore: 0.6,
			Constraints: []string{"企鹅", "飞"},
		},
		{
			Name:     "game_worldbuilding",
			Category: "story",
			Prompt:   "为一款科幻 RPG 游戏设计世界观背景，包含文明设定、冲突来源和主要势力，800 字以内",
			MinScore: 0.5,
			Constraints: []string{"科幻", "文明"},
		},
		{
			Name:     "serial_story_episode",
			Category: "story",
			Prompt:   "写一个悬疑系列短剧的第一集梗概，要在结尾留下悬念，引导观众看下一集",
			MinScore: 0.6,
			Constraints: []string{"悬疑", "悬念"},
		},

		// === 创意发散 (5 cases) ===
		{
			Name:     "brainstorm_video_hooks",
			Category: "divergent",
			Prompt:   "为一个美食账号生成 5 个不同风格的短视频开场 hook，每个不超过 3 秒的画面描述",
			MinScore: 0.5,
			Constraints: []string{"5", "美食"},
		},
		{
			Name:     "naming_alternatives",
			Category: "divergent",
			Prompt:   "为一款面向创意工作者的 AI 工具起 8 个候选名称，要求中英文各 4 个，附简短释义",
			MinScore: 0.5,
			Constraints: []string{"8", "AI"},
		},
		{
			Name:     "visual_style_exploration",
			Category: "divergent",
			Prompt:   "为一个音乐节海报提出 4 种完全不同的视觉风格方案，每种包含配色、字体、构图描述",
			MinScore: 0.6,
			Constraints: []string{"4", "音乐节"},
		},
		{
			Name:     "content_calendar",
			Category: "divergent",
			Prompt:   "为一个旅行博主规划一周 7 天的内容日历，每天一个主题，覆盖图文、短视频、直播三种形式",
			MinScore: 0.6,
			Constraints: []string{"7", "旅行"},
		},
		{
			Name:     "transition_ideas",
			Category: "divergent",
			Prompt:   "为一个 vlog 剪辑提供 6 种创意转场方式，不使用常见的淡入淡出和硬切",
			MinScore: 0.5,
			Constraints: []string{"6", "转场"},
		},
	}
}

func TestEval_CreativeWriting(t *testing.T) {
	testutil.RequireIntegration(t)

	suite := &eval.EvalSuite{Name: "creative_writing"}
	rt := eval.NewLLMRuntime(t, "",
		eval.WithSystemPrompt("You are a professional creative writer and content strategist. Produce high-quality creative content that meets all specified constraints. Always respond in Chinese."),
	)
	judgeMdl := eval.NewLLMModel(t, "")
	judge := &eval.LLMJudge{Model: judgeMdl, MaxRetries: 2}

	for _, tc := range cases() {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			resp, err := rt.Run(context.Background(), api.Request{
				Prompt:    tc.Prompt,
				SessionID: "creative-writing-" + tc.Name,
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
					"category":   tc.Category,
					"dimensions": result.Dimensions,
					"reasoning":  result.Reasoning,
				},
			})

			if !pass {
				t.Logf("case %q [%s]: overall=%.2f (min=%.2f)",
					tc.Name, tc.Category, result.Overall, tc.MinScore)
			}
		})
	}

	t.Cleanup(func() {
		t.Logf("\n%s", suite.Summary())
		if suite.PassRate() < 0.60 {
			t.Errorf("creative_writing pass rate %.1f%% below 60%% threshold", suite.PassRate()*100)
		}
	})
}
