//go:build integration

package creative_tool_selection_eval

import "github.com/saker-ai/saker/pkg/model"

func creativeToolCases() []toolSelectionCase {
	return []toolSelectionCase{
		// === 媒体分析组 (8 cases) ===
		{
			Name:         "video_sample_timestamp",
			Prompt:       "从视频 demo.mp4 中提取第 30 秒的画面",
			ExpectedTool: "video_sampler",
			ExpectedParams: map[string]string{
				"timestamp": "30",
			},
		},
		{
			Name:         "video_sample_interval",
			Prompt:       "每隔 5 秒从 interview.mp4 中截取一帧关键画面",
			ExpectedTool: "video_sampler",
			ExpectedParams: map[string]string{
				"interval": "5",
			},
		},
		{
			Name:          "video_summarize",
			Prompt:        "总结一下 lecture.mp4 这个视频的主要内容",
			ExpectedTool:  "video_summarizer",
			AcceptedTools: []string{"analyze_video"},
		},
		{
			Name:         "frame_analyze_content",
			Prompt:       "分析这张视频截图中的人物表情和场景布置",
			ExpectedTool: "frame_analyzer",
			AcceptedTools: []string{"image_read"},
		},
		{
			Name:         "analyze_video_full",
			Prompt:       "对 product_demo.mp4 进行完整的内容分析，识别产品展示的关键时刻",
			ExpectedTool: "analyze_video",
			AcceptedTools: []string{"video_summarizer"},
		},
		{
			Name:         "video_sample_multiple",
			Prompt:       "提取 trailer.mp4 中 00:10、00:30、01:00 三个时间点的画面",
			ExpectedTool: "video_sampler",
		},
		{
			Name:         "frame_analyze_composition",
			Prompt:       "分析这个镜头的构图方式和色彩运用",
			ExpectedTool: "frame_analyzer",
		},
		{
			Name:          "video_summarize_short",
			Prompt:        "用三句话概括 vlog.mp4 的内容",
			ExpectedTool:  "video_summarizer",
			AcceptedTools: []string{"analyze_video"},
		},

		// === 媒体管理组 (6 cases) ===
		{
			Name:         "stream_capture_live",
			Prompt:       "捕获直播流 rtmp://live.example.com/stream 的当前画面",
			ExpectedTool: "stream_capture",
			ExpectedParams: map[string]string{
				"url": "rtmp://",
			},
		},
		{
			Name:         "stream_monitor_status",
			Prompt:       "监控直播流的连接状态和帧率",
			ExpectedTool: "stream_monitor",
		},
		{
			Name:         "media_index_build",
			Prompt:       "为 assets/ 目录下的所有媒体文件建立索引",
			ExpectedTool: "media_index",
			ExpectedParams: map[string]string{
				"path": "assets",
			},
		},
		{
			Name:         "media_search_keyword",
			Prompt:       "在媒体库中搜索包含'日落'场景的视频片段",
			ExpectedTool: "media_search",
			ExpectedParams: map[string]string{
				"query": "日落",
			},
		},
		{
			Name:          "image_read_analyze",
			Prompt:        "读取 poster.png 并描述图片内容",
			ExpectedTool:  "image_read",
			AcceptedTools: []string{"read"},
		},
		{
			Name:         "media_search_filter",
			Prompt:       "找出所有时长超过 30 秒的 1080p 视频文件",
			ExpectedTool: "media_search",
		},

		// === 画布操作组 (6 cases) ===
		{
			Name:         "canvas_get_node",
			Prompt:       "获取画布中 id 为 node_123 的元素信息",
			ExpectedTool: "canvas_get_node",
			ExpectedParams: map[string]string{
				"node_id": "node_123",
			},
		},
		{
			Name:         "canvas_list_all",
			Prompt:       "列出画布上所有的文本节点",
			ExpectedTool: "canvas_list_nodes",
		},
		{
			Name:         "canvas_table_write",
			Prompt:       "在画布的表格中写入第 2 行第 3 列的数据为 '完成'",
			ExpectedTool: "canvas_table_write",
			ExpectedParams: map[string]string{
				"row": "2",
			},
		},
		{
			Name:         "canvas_list_filter",
			Prompt:       "找出画布上所有的图片元素",
			ExpectedTool: "canvas_list_nodes",
		},
		{
			Name:         "canvas_get_specific",
			Prompt:       "查看画布上标题节点的当前内容和位置",
			ExpectedTool: "canvas_get_node",
		},
		{
			Name:         "canvas_table_update",
			Prompt:       "更新画布表格的标题行",
			ExpectedTool: "canvas_table_write",
		},

		// === Agent 编排组 (6 cases) ===
		{
			Name:         "spawn_agent_research",
			Prompt:       "创建一个子 Agent 来专门负责调研竞品的视频风格",
			ExpectedTool: "spawn_agent",
		},
		{
			Name:         "spawn_agents_batch",
			Prompt:       "同时启动 3 个 Agent 分别处理视频剪辑、字幕生成和配乐选择",
			ExpectedTool: "spawn_agents_batch",
			AcceptedTools: []string{"spawn_agent"},
		},
		{
			Name:         "task_create_project",
			Prompt:       "创建一个任务：完成产品宣传片的脚本初稿",
			ExpectedTool: "task_create",
			AcceptedTools: []string{"todo_write"},
		},
		{
			Name:         "todo_write_checklist",
			Prompt:       "记录一下接下来要做的事：1. 选素材 2. 剪辑 3. 调色 4. 加字幕",
			ExpectedTool: "todo_write",
			AcceptedTools: []string{"task_create"},
		},
		{
			Name:         "spawn_agent_creative",
			Prompt:       "派一个 Agent 去生成三个不同风格的开场方案",
			ExpectedTool: "spawn_agent",
		},
		{
			Name:         "task_create_deadline",
			Prompt:       "添加一个紧急任务：今天内完成片头动画设计",
			ExpectedTool: "task_create",
			AcceptedTools: []string{"todo_write"},
		},

		// === 多工具链组 (8 cases) ===
		{
			Name:       "chain_extract_analyze",
			Prompt:     "从 demo.mp4 中提取第 15 秒的画面，然后分析画面中的产品摆放",
			IsChain:    true,
			ChainTools: []string{"video_sampler", "frame_analyzer"},
			ExpectedTool: "video_sampler",
		},
		{
			Name:       "chain_search_summarize",
			Prompt:     "在媒体库中搜索所有关于'城市夜景'的素材，然后生成一份内容摘要",
			IsChain:    true,
			ChainTools: []string{"media_search", "video_summarizer"},
			ExpectedTool: "media_search",
		},
		{
			Name:       "chain_capture_analyze",
			Prompt:     "捕获直播流画面，分析当前画面的构图质量",
			IsChain:    true,
			ChainTools: []string{"stream_capture", "frame_analyzer"},
			ExpectedTool: "stream_capture",
		},
		{
			Name:       "chain_sample_index",
			Prompt:     "对 raw_footage.mp4 每 10 秒采样一帧，然后为所有采样帧建立索引",
			IsChain:    true,
			ChainTools: []string{"video_sampler", "media_index"},
			ExpectedTool: "video_sampler",
		},
		{
			Name:       "chain_canvas_spawn",
			Prompt:     "获取画布上的故事板节点，然后为每个场景创建独立的 Agent 进行细化",
			IsChain:    true,
			ChainTools: []string{"canvas_list_nodes", "spawn_agents_batch"},
			ExpectedTool: "canvas_list_nodes",
		},
		{
			Name:       "chain_analyze_task",
			Prompt:     "分析 tutorial.mp4 的内容结构，然后为每个章节创建剪辑任务",
			IsChain:    true,
			ChainTools: []string{"analyze_video", "task_create"},
			ExpectedTool: "analyze_video",
		},
		{
			Name:       "chain_search_canvas",
			Prompt:     "搜索所有'产品特写'素材，把搜索结果更新到画布表格中",
			IsChain:    true,
			ChainTools: []string{"media_search", "canvas_table_write"},
			ExpectedTool: "media_search",
		},
		{
			Name:       "chain_monitor_capture_analyze",
			Prompt:     "检查直播流状态，如果正常就截取当前画面并分析画质",
			IsChain:    true,
			ChainTools: []string{"stream_monitor", "stream_capture", "frame_analyzer"},
			ExpectedTool: "stream_monitor",
		},
	}
}

func creativeToolDefinitions() []model.ToolDefinition {
	return []model.ToolDefinition{
		// 基础工具 (干扰项)
		{
			Name:        "bash",
			Description: "Execute a shell command",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"command": map[string]any{"type": "string", "description": "The command to execute"}},
				"required":   []string{"command"},
			},
		},
		{
			Name:        "read",
			Description: "Read a file from the filesystem",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"file_path": map[string]any{"type": "string", "description": "The file path to read"}},
				"required":   []string{"file_path"},
			},
		},
		{
			Name:        "write",
			Description: "Write content to a file",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"file_path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}},
				"required":   []string{"file_path", "content"},
			},
		},
		{
			Name:        "grep",
			Description: "Search file contents using regex patterns",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"pattern": map[string]any{"type": "string", "description": "Regex pattern to search"}},
				"required":   []string{"pattern"},
			},
		},
		{
			Name:        "glob",
			Description: "Find files matching a glob pattern",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{"pattern": map[string]any{"type": "string", "description": "Glob pattern"}},
				"required":   []string{"pattern"},
			},
		},
		// 媒体分析工具
		{
			Name:        "video_sampler",
			Description: "Extract frames from a video at specified timestamps or intervals. Use for capturing specific moments or creating frame sequences.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":      map[string]any{"type": "string", "description": "Video file path"},
					"timestamp": map[string]any{"type": "string", "description": "Timestamp to extract (seconds or HH:MM:SS)"},
					"interval":  map[string]any{"type": "string", "description": "Interval between frames in seconds"},
					"count":     map[string]any{"type": "integer", "description": "Number of frames to extract"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "video_summarizer",
			Description: "Generate a text summary of video content including key scenes, topics discussed, and timeline overview.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":   map[string]any{"type": "string", "description": "Video file path"},
					"detail": map[string]any{"type": "string", "description": "Summary detail level: brief, standard, detailed"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "frame_analyzer",
			Description: "Analyze a single image/frame for visual content including objects, people, composition, colors, and scene description.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"image":  map[string]any{"type": "string", "description": "Image file path or frame data"},
					"aspect": map[string]any{"type": "string", "description": "Analysis focus: composition, color, objects, people, scene"},
				},
				"required": []string{"image"},
			},
		},
		{
			Name:        "analyze_video",
			Description: "Perform comprehensive video analysis including scene detection, content categorization, key moments, and visual quality assessment.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":     map[string]any{"type": "string", "description": "Video file path"},
					"analysis": map[string]any{"type": "string", "description": "Analysis type: full, scenes, quality, content"},
				},
				"required": []string{"file"},
			},
		},
		// 媒体管理工具
		{
			Name:        "stream_capture",
			Description: "Capture a snapshot or short clip from a live stream URL (RTMP, HLS, WebRTC).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":      map[string]any{"type": "string", "description": "Stream URL (rtmp://, http://, etc.)"},
					"duration": map[string]any{"type": "integer", "description": "Capture duration in seconds (0 for snapshot)"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "stream_monitor",
			Description: "Monitor a live stream's health metrics: connection status, frame rate, bitrate, latency.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":     map[string]any{"type": "string", "description": "Stream URL to monitor"},
					"metrics": map[string]any{"type": "array", "description": "Metrics to check: fps, bitrate, latency, status"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "media_index",
			Description: "Build or update a searchable index of media files in a directory. Indexes metadata, thumbnails, and content tags.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Directory path to index"},
					"refresh": map[string]any{"type": "boolean", "description": "Force re-index existing files"},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "media_search",
			Description: "Search the media library by content description, tags, technical properties, or metadata.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":  map[string]any{"type": "string", "description": "Search query (natural language or tags)"},
					"filter": map[string]any{"type": "object", "description": "Filters: type, duration, resolution, format"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "image_read",
			Description: "Read and analyze an image file, returning visual description and metadata (dimensions, format, color profile).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Image file path"},
				},
				"required": []string{"file_path"},
			},
		},
		// 画布工具
		{
			Name:        "canvas_get_node",
			Description: "Get details of a specific node on the canvas by its ID, including position, content, style, and connections.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{"type": "string", "description": "The node ID to retrieve"},
				},
				"required": []string{"node_id"},
			},
		},
		{
			Name:        "canvas_list_nodes",
			Description: "List all nodes on the canvas, optionally filtered by type (text, image, shape, group, table).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":   map[string]any{"type": "string", "description": "Filter by node type"},
					"parent": map[string]any{"type": "string", "description": "Filter by parent group ID"},
				},
			},
		},
		{
			Name:        "canvas_table_write",
			Description: "Write data to a specific cell or range in a canvas table element.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_id": map[string]any{"type": "string", "description": "Table node ID"},
					"row":      map[string]any{"type": "string", "description": "Row index (0-based)"},
					"col":      map[string]any{"type": "string", "description": "Column index (0-based)"},
					"value":    map[string]any{"type": "string", "description": "Value to write"},
				},
				"required": []string{"table_id", "row", "col", "value"},
			},
		},
		// Agent 编排工具
		{
			Name:        "spawn_agent",
			Description: "Create and start a new child agent with a specific role and task description.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role":    map[string]any{"type": "string", "description": "Agent role/persona"},
					"task":    map[string]any{"type": "string", "description": "Task description for the agent"},
					"tools":   map[string]any{"type": "array", "description": "Tools available to the child agent"},
				},
				"required": []string{"task"},
			},
		},
		{
			Name:        "spawn_agents_batch",
			Description: "Create multiple child agents simultaneously, each with their own role and task.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agents": map[string]any{"type": "array", "description": "Array of agent configs with role and task"},
				},
				"required": []string{"agents"},
			},
		},
		{
			Name:        "task_create",
			Description: "Create a new task in the task tracking system with title, description, priority, and deadline.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string", "description": "Task title"},
					"description": map[string]any{"type": "string", "description": "Task description"},
					"priority":    map[string]any{"type": "string", "description": "Priority: low, medium, high, urgent"},
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "todo_write",
			Description: "Write a quick to-do list or checklist for tracking items to complete.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items": map[string]any{"type": "array", "description": "List of to-do items"},
					"title": map[string]any{"type": "string", "description": "Optional list title"},
				},
				"required": []string{"items"},
			},
		},
	}
}
