package api

import "testing"

func TestDetectLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"chinese", "你好世界，今天天气怎么样？", "Chinese"},
		{"english", "Hello world, how are you today?", "English"},
		{"japanese", "こんにちは世界、元気ですか？", "Japanese"},
		{"korean", "안녕하세요 세계, 오늘 기분이 어때요?", "Korean"},
		{"french", "Bonjour le monde, comment allez-vous aujourd'hui?", "French"},
		{"german", "Hallo Welt, wie geht es Ihnen heute?", "German"},
		{"spanish", "Hola mundo, cómo estás hoy?", "Spanish"},
		{"russian", "Привет мир, как дела сегодня?", "Russian"},
		{"arabic", "مرحبا بالعالم، كيف حالك اليوم؟", "Arabic"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != tc.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDetectLanguageCJKShort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		input string
		want string
	}{
		{"chinese_3", "你好吗", "Chinese"},
		{"chinese_4", "帮我画图", "Chinese"},
		{"chinese_6", "你好世界帮我", "Chinese"},
		{"chinese_question", "今天天气如何", "Chinese"},
		{"chinese_command", "生成一张图片", "Chinese"},
		{"japanese_3", "おはよう", "Japanese"},
		{"japanese_hiragana", "ありがとうございます", "Japanese"},
		{"korean_3", "안녕하세요", "Korean"},
		{"korean_short", "감사합니다", "Korean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != tc.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDetectLanguageTooShort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"2_latin", "hi"},
		{"2_latin_ok", "ok"},
		{"1_char", "a"},
		{"2_chinese", "你好"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != "" {
				t.Errorf("detectLanguage(%q) = %q, want empty (below min runes)", tc.input, got)
			}
		})
	}
}

func TestDetectLanguageLatinAmbiguous(t *testing.T) {
	t.Parallel()
	// Short Latin-script words that are ambiguous across languages.
	// Should return empty or English — never a wrong non-English language.
	tests := []struct {
		name  string
		input string
	}{
		{"server", "server"},
		{"server_start", "server start"},
		{"run_build_test", "run build test"},
		{"hello", "hello"},
		{"data", "data"},
		{"status", "status"},
		{"config", "config"},
		{"menu", "menu"},
		{"image", "image"},
		{"file_path", "/usr/local/bin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != "" && got != "English" {
				t.Errorf("detectLanguage(%q) = %q, want empty or English (not a wrong language)", tc.input, got)
			}
		})
	}
}

func TestDetectLanguageLongerSentences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"chinese_long", "请帮我生成一张美丽的风景图片，要有山有水有蓝天白云", "Chinese"},
		{"english_long", "Please help me generate a beautiful landscape image with mountains and rivers", "English"},
		{"french_long", "S'il vous plaît, aidez-moi à générer une belle image de paysage", "French"},
		{"german_long", "Bitte helfen Sie mir, ein schönes Landschaftsbild zu erstellen", "German"},
		{"spanish_long", "Por favor, ayúdame a generar una hermosa imagen de paisaje", "Spanish"},
		{"russian_long", "Пожалуйста, помогите мне создать красивое изображение пейзажа", "Russian"},
		{"arabic_long", "من فضلك ساعدني في إنشاء صورة جميلة للمناظر الطبيعية", "Arabic"},
		{"japanese_long", "美しい風景の画像を生成するのを手伝ってください", "Japanese"},
		{"korean_long", "아름다운 풍경 이미지를 생성하도록 도와주세요", "Korean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != tc.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDetectLanguageMixedContent(t *testing.T) {
	t.Parallel()
	// Text with mixed scripts — should detect the dominant language.
	tests := []struct {
		name     string
		input    string
		want     string
		allowAlt string // alternative acceptable result
	}{
		{"chinese_with_english_words", "帮我用Python写一个hello world程序", "Chinese", ""},
		{"chinese_with_url", "请访问 https://example.com 这个网站", "Chinese", ""},
		{"english_with_code", "Please fix the bug in function handleRequest", "English", ""},
		{"japanese_with_english", "このプログラムのバグを修正してください", "Japanese", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if got != tc.want && (tc.allowAlt == "" || got != tc.allowAlt) {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDetectLanguageSpecialInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantEmpty bool
	}{
		{"only_numbers", "123456789", true},
		{"only_punctuation", "...!!??", true},
		{"emoji_only", "😀🎉🚀🌟", true},
		{"spaces_only", "       ", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detectLanguage(tc.input)
			if tc.wantEmpty && got != "" {
				t.Errorf("detectLanguage(%q) = %q, want empty", tc.input, got)
			}
		})
	}
}

func TestDetectLanguageTruncatesLongInput(t *testing.T) {
	t.Parallel()
	long := ""
	for i := 0; i < 500; i++ {
		long += "中"
	}
	got := detectLanguage(long)
	if got != "Chinese" {
		t.Errorf("expected Chinese for long input, got %q", got)
	}
}
