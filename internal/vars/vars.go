// Package vars 提供全局变量管理功能
// 支持变量的设置、获取和字符串替换
package vars

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	"go.uber.org/zap"

	"pipetGo/internal/logger"
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
// text: 包含变量引用的字符串（格式: {{var}} 或 {{VarName}}）
// 返回: 替换后的字符串
// 支持的变量命名格式：
//   - 蛇形命名: user_id, authorization_token
//   - 驼峰命名: userId, authorizationToken
//   - 帕斯卡命名: UserId, AuthorizationToken
func Replace(text string) string {
	if text == "" {
		return text
	}

	re := regexp.MustCompile(`\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}`)
	result := re.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		
		varsMu.Lock()
		// 1. 首先尝试完全匹配
		if value, ok := vars[key]; ok {
			varsMu.Unlock()
			logger.Debug("变量替换命中", zap.String("key", key), zap.String("value", maskValue(value)))
			return value
		}
		
		// 2. 尝试大小写不敏感匹配（驼峰 vs 蛇形）
		lowerKey := strings.ToLower(key)
		for k, v := range vars {
			if strings.ToLower(k) == lowerKey {
				varsMu.Unlock()
				logger.Debug("变量替换命中（大小写不敏感）", zap.String("key", key), zap.String("actual_key", k), zap.String("value", maskValue(v)))
				return v
			}
			
			// 3. 尝试蛇形/驼峰转换匹配
			if snakeToCamel(k) == key || camelToSnake(k) == key {
				varsMu.Unlock()
				logger.Debug("变量替换命中（命名格式转换）", zap.String("key", key), zap.String("actual_key", k), zap.String("value", maskValue(v)))
				return v
			}
		}
		varsMu.Unlock()
		
		logger.Debug("变量未找到，保留原样", zap.String("key", key), zap.String("text", text))
		return match
	})

	if result != text {
		logger.Debug("Replace 完成", zap.String("before", text), zap.String("after", maskValue(result)))
	}
	return result
}

// snakeToCamel 将蛇形命名转换为驼峰命名
// 例如: user_id -> userId
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	result := ""
	for i, part := range parts {
		if i == 0 {
			result += part
		} else {
			result += strings.Title(part)
		}
	}
	return result
}

// camelToSnake 将驼峰命名转换为蛇形命名
// 例如: userId -> user_id
func camelToSnake(s string) string {
	var result strings.Builder
	for i, c := range s {
		if i > 0 && unicode.IsUpper(c) {
			result.WriteByte('_')
		}
		result.WriteRune(unicode.ToLower(c))
	}
	return result.String()
}

// maskValue 对长度较长的值做掩码，避免日志泄露完整 token
func maskValue(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "***" + s[len(s)-6:]
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