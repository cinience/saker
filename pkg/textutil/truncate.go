package textutil

// TruncateRunes returns s capped to max runes without adding a suffix.
func TruncateRunes(s string, max int) string {
	r := []rune(s)
	if max < 0 {
		max = 0
	}
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// TruncateBytes returns s capped to max bytes without splitting UTF-8 runes.
func TruncateBytes(s string, max int) string {
	if max < 0 {
		max = 0
	}
	if len(s) <= max {
		return s
	}
	for i := range s {
		if i > max {
			break
		}
		if i == max {
			return s[:i]
		}
	}
	end := 0
	for i := range s {
		if i > max {
			break
		}
		end = i
	}
	return s[:end]
}

// TailRunes returns the last max runes from s.
func TailRunes(s string, max int) string {
	r := []rune(s)
	if max < 0 {
		max = 0
	}
	if len(r) <= max {
		return s
	}
	return string(r[len(r)-max:])
}

// TruncateRunesAfter returns s capped to max runes, then appends suffix.
// The suffix is extra; max describes only the number of runes kept from s.
func TruncateRunesAfter(s string, max int, suffix string) string {
	r := []rune(s)
	if max < 0 {
		max = 0
	}
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + suffix
}

// TruncateRunesWithin returns s capped to max total runes, including suffix.
func TruncateRunesWithin(s string, max int, suffix string) string {
	r := []rune(s)
	if max < 0 {
		max = 0
	}
	if len(r) <= max {
		return s
	}
	if max == 0 {
		return ""
	}
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= max {
		return string(suffixRunes[:max])
	}
	return string(r[:max-len(suffixRunes)]) + suffix
}
