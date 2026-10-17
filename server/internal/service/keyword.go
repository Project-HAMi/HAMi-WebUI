package service

import "strings"

var fieldSeparators = strings.NewReplacer("-", "", "_", "")

// matchesKeyword reports whether every space-separated word of keyword appears within
// one field, ignoring case. Fields also match without "-" and "_", while separators typed
// in a word stay literal. This mirrors packages/web/src/utils/keyword-search.mjs.
func matchesKeyword(keyword string, fields ...string) bool {
	words := strings.Fields(strings.ToLower(keyword))
	if len(words) == 0 {
		return true
	}
	texts := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		plain := strings.ToLower(field)
		texts = append(texts, plain, fieldSeparators.Replace(plain))
	}
	for _, word := range words {
		found := false
		for _, text := range texts {
			if strings.Contains(text, word) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
