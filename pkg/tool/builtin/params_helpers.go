package toolbuiltin

import (
	"fmt"
	"strings"
)

func requiredString(params map[string]interface{}, key string) (string, error) {
	raw, ok := params[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	value, err := coerceString(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be string: %w", key, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", key)
	}
	return value, nil
}
