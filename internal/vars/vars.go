package vars

import (
	"regexp"
	"strings"
	"sync"
)

var (
	vars   = make(map[string]string)
	varsMu sync.Mutex
)

func Set(key, value string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	vars[key] = value
}

func Get(key string) string {
	varsMu.Lock()
	defer varsMu.Unlock()
	return vars[key]
}

func GetAll() map[string]string {
	varsMu.Lock()
	defer varsMu.Unlock()
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = v
	}
	return result
}

func Replace(text string) string {
	if text == "" {
		return text
	}

	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if value, ok := vars[key]; ok {
			return value
		}
		return match
	})
}

func InitFromConfig(config map[string]string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	for k, v := range config {
		vars[k] = v
	}
}
