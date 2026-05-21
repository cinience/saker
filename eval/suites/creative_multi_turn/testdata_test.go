//go:build integration

package creative_multi_turn_eval

func creativeMultiTurnCases() []creativeMultiTurnCase {
	return []creativeMultiTurnCase{
		// === 创作上下文保持 (4 cases) ===
		{
			Name:      "remember_video_params",
			SessionID: "cmt-video-params",
			Turns: []creativeTurn{
				{Prompt: "我要制作一个 16:9 比例、1080p、30fps 的产品宣传视频"},
				{Prompt: "刚才说的视频分辨率是多少？", ExpectedContains: []string{"1080", "1920"}},
				{Prompt: "帧率呢？", ExpectedContains: []string{"30"}},
				{Prompt: "宽高比是什么？", ExpectedContains: []string{"16:9", "16：9"}},
			},
		},
		{
			Name:      "remember_style_choices",
			SessionID: "cmt-style",
			Turns: []creativeTurn{
				{Prompt: "这个项目的视觉风格定为赛博朋克，主色调用霓虹紫和电光蓝"},
				{Prompt: "我们定的什么风格？", ExpectedContains: []string{"赛博朋克", "赛博"}},
				{Prompt: "主色调是什么？", ExpectedContains: []string{"紫", "蓝"}},
			},
		},
		{
			Name:      "remember_character_names",
			SessionID: "cmt-characters",
			Turns: []creativeTurn{
				{Prompt: "脚本里有三个角色：主角叫林夕，反派叫陈刚，配角叫小美"},
				{Prompt: "主角的名字是？", ExpectedContains: []string{"林夕"}},
				{Prompt: "谁是反派？", ExpectedContains: []string{"陈刚"}},
				{Prompt: "还有一个配角叫什么？", ExpectedContains: []string{"小美"}},
			},
		},
		{
			Name:      "remember_project_timeline",
			SessionID: "cmt-timeline",
			Turns: []creativeTurn{
				{Prompt: "项目周期 3 周，第一周做脚本，第二周拍摄，第三周后期"},
				{Prompt: "第二周做什么？", ExpectedContains: []string{"拍摄"}},
				{Prompt: "整个项目多长时间？", ExpectedContains: []string{"3", "三"}},
			},
		},

		// === 指代消解 (4 cases) ===
		{
			Name:      "resolve_that_video",
			SessionID: "cmt-ref-video",
			Turns: []creativeTurn{
				{Prompt: "我上传了一个叫 sunset_beach.mp4 的视频"},
				{Prompt: "帮我分析一下那个视频的色调", ExpectedContains: []string{"sunset_beach", "sunset", "视频"}},
			},
		},
		{
			Name:      "resolve_previous_design",
			SessionID: "cmt-ref-design",
			Turns: []creativeTurn{
				{Prompt: "第一版封面设计用了渐变蓝色背景和白色标题"},
				{Prompt: "把它的背景改成红色，其他保持不变", ExpectedContains: []string{"红"}},
				{Prompt: "现在的设计是什么样的？", ExpectedContains: []string{"红", "白"}},
			},
		},
		{
			Name:      "resolve_last_suggestion",
			SessionID: "cmt-ref-suggest",
			Turns: []creativeTurn{
				{Prompt: "给我三个片头方案"},
				{Prompt: "我选第二个方案，在这个基础上加入粒子特效", ExpectedContains: []string{"第二", "粒子"}},
			},
		},
		{
			Name:      "resolve_that_scene",
			SessionID: "cmt-ref-scene",
			Turns: []creativeTurn{
				{Prompt: "第三幕有一个追逐戏，在雨夜的胡同里"},
				{Prompt: "那场追逐戏的时长大概需要多少？", ExpectedContains: []string{"追逐", "胡同"}},
			},
		},

		// === 意图澄清 (4 cases) ===
		{
			Name:      "clarify_ambiguous_edit",
			SessionID: "cmt-clarify-edit",
			Turns: []creativeTurn{
				{Prompt: "把那个改一下"},
				{Prompt: "我说的是标题的字体大小，改成 24px", ExpectedContains: []string{"24", "字体"}},
			},
		},
		{
			Name:      "clarify_vague_style",
			SessionID: "cmt-clarify-style",
			Turns: []creativeTurn{
				{Prompt: "风格要好看一点"},
				{Prompt: "具体来说，我想要电影感的调色，暗角+高对比度", ExpectedContains: []string{"电影", "暗角", "对比"}},
			},
		},
		{
			Name:      "clarify_format_requirement",
			SessionID: "cmt-clarify-format",
			Turns: []creativeTurn{
				{Prompt: "帮我导出视频"},
				{Prompt: "导出为 MP4 格式，H.265 编码，码率 8Mbps", ExpectedContains: []string{"mp4", "h.265", "h265", "265"}},
			},
		},
		{
			Name:      "clarify_target_platform",
			SessionID: "cmt-clarify-platform",
			Turns: []creativeTurn{
				{Prompt: "做一个短视频"},
				{Prompt: "是发抖音的，竖屏 9:16，15 秒以内", ExpectedContains: []string{"9:16", "9：16", "15", "竖"}},
			},
		},

		// === 创作迭代修改 (4 cases) ===
		{
			Name:      "iterative_color_change",
			SessionID: "cmt-iter-color",
			Turns: []creativeTurn{
				{Prompt: "背景用白色"},
				{Prompt: "不，改成浅灰色", ExpectedContains: []string{"灰"}},
				{Prompt: "再暗一点，用深灰", ExpectedContains: []string{"深灰"}},
				{Prompt: "最终背景色是什么？", ExpectedContains: []string{"深灰"}},
			},
		},
		{
			Name:      "iterative_text_refinement",
			SessionID: "cmt-iter-text",
			Turns: []creativeTurn{
				{Prompt: "标语写'科技改变生活'"},
				{Prompt: "太平淡了，改成更有力量感的", ExpectedContains: []string{}},
				{Prompt: "在上一版基础上加上'未来已来'的概念", ExpectedContains: []string{"未来"}},
			},
		},
		{
			Name:      "iterative_pacing_adjust",
			SessionID: "cmt-iter-pace",
			Turns: []creativeTurn{
				{Prompt: "开场 5 秒展示 logo，然后 10 秒产品特写"},
				{Prompt: "logo 展示太长了，缩短到 3 秒", ExpectedContains: []string{"3"}},
				{Prompt: "产品特写也缩短到 8 秒", ExpectedContains: []string{"8"}},
				{Prompt: "现在总共多少秒？", ExpectedContains: []string{"11"}},
			},
		},
		{
			Name:      "iterative_music_selection",
			SessionID: "cmt-iter-music",
			Turns: []creativeTurn{
				{Prompt: "配乐用轻快的电子乐"},
				{Prompt: "太嗨了，换成 lo-fi 风格", ExpectedContains: []string{"lo-fi", "lofi", "lo fi"}},
				{Prompt: "节奏再慢一点，BPM 在 70-80 之间", ExpectedContains: []string{"70", "80"}},
			},
		},

		// === 创作约束累积 (4 cases) ===
		{
			Name:      "accumulate_video_constraints",
			SessionID: "cmt-acc-video",
			Turns: []creativeTurn{
				{Prompt: "视频要 4K 分辨率"},
				{Prompt: "帧率要 60fps"},
				{Prompt: "编码用 H.265"},
				{Prompt: "色彩空间用 Rec.709"},
				{Prompt: "总结一下目前的所有技术规格", ExpectedContains: []string{"4k", "4K", "60", "265", "709"}},
			},
		},
		{
			Name:      "accumulate_brand_guidelines",
			SessionID: "cmt-acc-brand",
			Turns: []creativeTurn{
				{Prompt: "品牌色是 #FF6B35"},
				{Prompt: "字体统一用思源黑体"},
				{Prompt: "logo 必须在右上角"},
				{Prompt: "回顾一下品牌规范", ExpectedContains: []string{"FF6B35", "ff6b35", "思源", "右上"}},
			},
		},
		{
			Name:      "accumulate_scene_requirements",
			SessionID: "cmt-acc-scene",
			Turns: []creativeTurn{
				{Prompt: "第一场戏在咖啡厅"},
				{Prompt: "灯光要暖色调，模拟下午三点的阳光"},
				{Prompt: "背景要有轻柔的爵士乐"},
				{Prompt: "镜头用浅景深，光圈 f/1.8"},
				{Prompt: "汇总这场戏的所有要素", ExpectedContains: []string{"咖啡", "暖", "爵士", "1.8"}},
			},
		},
		{
			Name:      "accumulate_delivery_specs",
			SessionID: "cmt-acc-delivery",
			Turns: []creativeTurn{
				{Prompt: "交付格式：竖版 9:16"},
				{Prompt: "时长不超过 60 秒"},
				{Prompt: "要有中文字幕和英文字幕两版"},
				{Prompt: "文件大小控制在 50MB 以内"},
				{Prompt: "列出所有交付要求", ExpectedContains: []string{"9:16", "9：16", "60", "字幕", "50"}},
			},
		},
	}
}
