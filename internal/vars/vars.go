// Package vars 提供全局变量管理功能
// 支持变量的设置、获取和字符串替换
package vars

import (
	"regexp"
	"strings"
	"sync"
)

// vars 存储全局变量，使用 map 实现
var (
	vars   = make(map[string]string)
	varsMu sync.Mutex
)

// Set 设置全局变量
// key: 变量名
// value: 变量值
func Set(key, value string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	vars[key] = value
}

// Get 获取全局变量
// key: 变量名
// 返回: 变量值（如果不存在返回空字符串）
func Get(key string) string {
	varsMu.Lock()
	defer varsMu.Unlock()
	return vars[key]
}

// GetAll 获取所有全局变量
// 返回: 包含所有变量的 map 副本
func GetAll() map[string]string {
	varsMu.Lock()
	defer varsMu.Unlock()
	result := make(map[string]string)
	for k, v := range vars {
		result[k] = v
	}
	return result
}

// Delete 删除指定变量
// key: 变量名
func Delete(key string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	delete(vars, key)
}

// Replace 替换字符串中的变量引用
// text: 包含变量引用的字符串（格式: {{var}}）
// 返回: 替换后的字符串
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

// InitFromConfig 从配置初始化变量
// config: 配置 map
func InitFromConfig(config map[string]string) {
	varsMu.Lock()
	defer varsMu.Unlock()
	for k, v := range config {
		vars[k] = v
	}
}