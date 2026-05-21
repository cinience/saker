package api

import "github.com/pemistahl/lingua-go"

// langDetector is a package-level detector limited to common languages
// with full-accuracy mode and a minimum relative distance to reduce
// false positives on short or ambiguous inputs.
var langDetector = lingua.NewLanguageDetectorBuilder().
	FromLanguages(
		lingua.English, lingua.Chinese,
		lingua.Japanese, lingua.Korean,
		lingua.French, lingua.German, lingua.Spanish,
		lingua.Russian, lingua.Arabic,
	).
	WithMinimumRelativeDistance(0.15).
	WithPreloadedLanguageModels().
	Build()

const (
	langMinRunes      = 3
	langMinConfidence = 0.65
	langMaxSample     = 300
)

var linguaToName = map[lingua.Language]string{
	lingua.English:  "English",
	lingua.Chinese:  "Chinese",
	lingua.Japanese: "Japanese",
	lingua.Korean:   "Korean",
	lingua.French:   "French",
	lingua.German:   "German",
	lingua.Spanish:  "Spanish",
	lingua.Russian:  "Russian",
	lingua.Arabic:   "Arabic",
}

// detectLanguage detects the language of the given text.
// Returns a language name (e.g. "Chinese") or empty string when detection
// is uncertain or the input is too short for reliable classification.
func detectLanguage(text string) string {
	if len(text) == 0 {
		return ""
	}
	sample := []rune(text)
	if len(sample) < langMinRunes {
		return ""
	}
	if len(sample) > langMaxSample {
		sample = sample[:langMaxSample]
	}
	values := langDetector.ComputeLanguageConfidenceValues(string(sample))
	if len(values) == 0 {
		return ""
	}
	top := values[0]
	if top.Value() < langMinConfidence {
		return ""
	}
	if name, exists := linguaToName[top.Language()]; exists {
		return name
	}
	return ""
}
