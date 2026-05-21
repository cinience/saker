package creative_media_pipeline_eval

func mediaPipelineCases() []mediaPipelineCase {
	return []mediaPipelineCase{
		// === 视频处理管线 (4 cases) ===
		{
			Name:       "video_extract_keyframes",
			UserPrompt: "从产品演示视频中每 5 秒提取一帧关键画面，输出 1080p PNG",
			ExpectedSteps: []expectedStep{
				{Tool: "video_sampler", RequiredParams: map[string]string{"interval": "5"}},
				{Tool: "frame_analyzer", RequiredParams: map[string]string{}},
			},
			TechSpec: &techSpec{Resolution: "1080p", Format: "png"},
		},
		{
			Name:       "video_transcode_4k",
			UserPrompt: "将源视频转码为 4K H.265 60fps MP4",
			ExpectedSteps: []expectedStep{
				{Tool: "video_sampler", RequiredParams: map[string]string{"file": "source"}},
			},
			TechSpec: &techSpec{Resolution: "4k", FrameRate: 60, Codec: "h265", Format: "mp4"},
		},
		{
			Name:       "video_highlight_reel",
			UserPrompt: "分析视频内容，提取高光时刻，生成 30 秒精华片段",
			ExpectedSteps: []expectedStep{
				{Tool: "analyze_video", RequiredParams: map[string]string{"analysis": "scenes"}},
				{Tool: "video_sampler", RequiredParams: map[string]string{}},
			},
			TechSpec: &techSpec{Resolution: "1080p", FrameRate: 30, Codec: "h264", Format: "mp4"},
		},
		{
			Name:       "video_thumbnail_generation",
			UserPrompt: "为教学视频的每个章节生成缩略图",
			ExpectedSteps: []expectedStep{
				{Tool: "analyze_video", RequiredParams: map[string]string{"analysis": "scenes"}},
				{Tool: "video_sampler", RequiredParams: map[string]string{}},
				{Tool: "frame_analyzer", RequiredParams: map[string]string{}},
			},
			TechSpec: &techSpec{Format: "jpg"},
		},

		// === 直播流处理管线 (3 cases) ===
		{
			Name:       "stream_quality_check",
			UserPrompt: "检查 RTMP 直播流的画质和稳定性",
			ExpectedSteps: []expectedStep{
				{Tool: "stream_monitor", RequiredParams: map[string]string{"url": "rtmp://"}},
				{Tool: "stream_capture", RequiredParams: map[string]string{}},
				{Tool: "frame_analyzer", RequiredParams: map[string]string{}},
			},
		},
		{
			Name:       "stream_record_segment",
			UserPrompt: "录制直播流中的精彩片段，30 秒，1080p",
			ExpectedSteps: []expectedStep{
				{Tool: "stream_capture", RequiredParams: map[string]string{"duration": "30"}},
			},
			TechSpec: &techSpec{Resolution: "1080p", FrameRate: 30},
		},
		{
			Name:       "stream_multi_angle",
			UserPrompt: "同时捕获两路直播流画面进行对比",
			ExpectedSteps: []expectedStep{
				{Tool: "stream_capture", RequiredParams: map[string]string{}},
				{Tool: "stream_capture", RequiredParams: map[string]string{}},
				{Tool: "frame_analyzer", RequiredParams: map[string]string{}},
			},
		},

		// === 媒体库管理管线 (3 cases) ===
		{
			Name:       "media_catalog_build",
			UserPrompt: "扫描 assets 目录，建立完整的媒体资源索引",
			ExpectedSteps: []expectedStep{
				{Tool: "media_index", RequiredParams: map[string]string{"path": "assets"}},
			},
		},
		{
			Name:       "media_search_and_analyze",
			UserPrompt: "搜索所有日落场景素材，分析其色彩分布",
			ExpectedSteps: []expectedStep{
				{Tool: "media_search", RequiredParams: map[string]string{"query": "日落"}},
				{Tool: "frame_analyzer", RequiredParams: map[string]string{"aspect": "color"}},
			},
		},
		{
			Name:       "media_deduplicate",
			UserPrompt: "在媒体库中找出相似的重复素材",
			ExpectedSteps: []expectedStep{
				{Tool: "media_index", RequiredParams: map[string]string{}},
				{Tool: "media_search", RequiredParams: map[string]string{}},
			},
		},

		// === 错误处理管线 (2 cases) ===
		{
			Name:       "invalid_resolution_graceful",
			UserPrompt: "输出 999x999 分辨率的视频",
			ExpectedSteps: []expectedStep{
				{Tool: "video_sampler", RequiredParams: map[string]string{}},
			},
			TechSpec: &techSpec{Resolution: "1080p", Format: "mp4"},
		},
		{
			Name:       "invalid_framerate_fallback",
			UserPrompt: "生成 120fps 的慢动作视频",
			ExpectedSteps: []expectedStep{
				{Tool: "video_sampler", RequiredParams: map[string]string{}},
			},
			TechSpec: &techSpec{FrameRate: 60, Codec: "h264"},
		},
	}
}
