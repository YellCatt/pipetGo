package assert

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"pipetGo/internal/logger"
)

func BodyRegexMatch(body string, pattern string) (bool, string) {
	if pattern == "" {
		logger.Debug("响应体正则匹配: 空模式，跳过")
		return true, ""
	}

	negate := false
	if strings.HasPrefix(pattern, "!") {
		negate = true
		pattern = pattern[1:]
	}

	matched, err := regexp.MatchString(pattern, body)
	if err != nil {
		logger.Debug("响应体正则匹配: 正则匹配出错", zap.String("模式", pattern), zap.Error(err))
		return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
	}

	if negate {
		matched = !matched
		logger.Debug("响应体正则匹配: 取反模式", zap.String("模式", pattern), zap.Bool("结果", matched))
	} else {
		logger.Debug("响应体正则匹配: 正向模式", zap.String("模式", pattern), zap.Bool("结果", matched))
	}

	if !matched {
		if negate {
			return false, fmt.Sprintf("响应体不应包含模式 '%s'", pattern)
		}
		return false, fmt.Sprintf("响应体应包含模式 '%s'", pattern)
	}

	return true, ""
}

func JSONMatch(expected, actual string, matchMode string) (bool, string) {
	if expected == "" {
		logger.Debug("JSON 匹配: 期望值为空，跳过")
		return true, ""
	}

	expectedData := gjson.Parse(expected)
	actualData := gjson.Parse(actual)

	logger.Debug("JSON 匹配",
		zap.String("模式", matchMode),
		zap.String("期望值", truncateForLog(expected)),
		zap.Bool("期望是对象", expectedData.IsObject()),
		zap.Int("期望字段数", len(expectedData.Map())))

	if matchMode == "subset" {
		return jsonSubsetMatch(expectedData, actualData)
	}

	return jsonExactMatch(expectedData, actualData)
}

func jsonExactMatch(expected, actual gjson.Result) (bool, string) {
	if !expected.IsObject() || !actual.IsObject() {
		return compareValues(expected, actual)
	}

	expectedMap := expected.Map()
	actualMap := actual.Map()

	if len(expectedMap) != len(actualMap) {
		return false, fmt.Sprintf("期望 %d 个键，实际 %d 个键", len(expectedMap), len(actualMap))
	}

	for key, expectedVal := range expectedMap {
		actualVal, exists := actualMap[key]
		if !exists {
			return false, fmt.Sprintf("缺少键: %s", key)
		}

		if ok, err := compareValues(expectedVal, actualVal); !ok {
			return false, fmt.Sprintf("键 '%s': %s", key, err)
		}
	}

	return true, ""
}

func jsonSubsetMatch(expected, actual gjson.Result) (bool, string) {
	if !expected.IsObject() || !actual.IsObject() {
		return compareValues(expected, actual)
	}

	expectedMap := expected.Map()
	actualMap := actual.Map()

	for key, expectedVal := range expectedMap {
		actualVal, exists := actualMap[key]

		if expectedVal.Str == "{{not_exists}}" {
			if exists {
				return false, fmt.Sprintf("键 '%s' 不应存在", key)
			}
			continue
		}

		if !exists {
			return false, fmt.Sprintf("缺少键: %s", key)
		}

		if ok, err := compareValues(expectedVal, actualVal); !ok {
			return false, fmt.Sprintf("键 '%s': %s", key, err)
		}
	}

	return true, ""
}

func compareValues(expected, actual gjson.Result) (bool, string) {
	expectedStr := expected.Str
	expectedRaw := expected.Raw

	// 检查字符串类型的期望值
	if expected.Type == gjson.String {
		if expectedStr == "{{skip}}" {
			return true, ""
		}

		if strings.HasPrefix(expectedStr, "{{regex:") && strings.HasSuffix(expectedStr, "}}") {
			pattern := expectedStr[8 : len(expectedStr)-2]
			pattern = fixRegexEscapes(pattern)
			pattern, err := validateRegexPattern(pattern)
			if err != nil {
				return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
			}
			matched, err := regexp.MatchString(pattern, actual.String())
			if err != nil {
				return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
			}
			if !matched {
				return false, fmt.Sprintf("值 '%s' 不匹配正则 '%s'", actual.String(), pattern)
			}
			return true, ""
		}

		if strings.HasPrefix(expectedStr, "{{not_regex:") && strings.HasSuffix(expectedStr, "}}") {
			pattern := expectedStr[12 : len(expectedStr)-2]
			pattern = fixRegexEscapes(pattern)
			pattern, err := validateRegexPattern(pattern)
			if err != nil {
				return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
			}
			matched, err := regexp.MatchString(pattern, actual.String())
			if err != nil {
				return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
			}
			if matched {
				return false, fmt.Sprintf("值 '%s' 不应匹配正则 '%s'", actual.String(), pattern)
			}
			return true, ""
		}
	}

	// 检查非字符串类型的期望值（如数字、布尔等）
	if expectedRaw == "{{skip}}" {
		return true, ""
	}

	if strings.HasPrefix(expectedRaw, "{{regex:") && strings.HasSuffix(expectedRaw, "}}") {
		pattern := expectedRaw[8 : len(expectedRaw)-2]
		pattern = fixRegexEscapes(pattern)
		pattern, err := validateRegexPattern(pattern)
		if err != nil {
			return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
		}
		matched, err := regexp.MatchString(pattern, actual.String())
		if err != nil {
			return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
		}
		if !matched {
			return false, fmt.Sprintf("值 '%s' 不匹配正则 '%s'", actual.String(), pattern)
		}
		return true, ""
	}

	if strings.HasPrefix(expectedRaw, "{{not_regex:") && strings.HasSuffix(expectedRaw, "}}") {
		pattern := expectedRaw[12 : len(expectedRaw)-2]
		pattern = fixRegexEscapes(pattern)
		pattern, err := validateRegexPattern(pattern)
		if err != nil {
			return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
		}
		matched, err := regexp.MatchString(pattern, actual.String())
		if err != nil {
			return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
		}
		if matched {
			return false, fmt.Sprintf("值 '%s' 不应匹配正则 '%s'", actual.String(), pattern)
		}
		return true, ""
	}

	// 常规类型和值比较
	if expected.Type != actual.Type {
		return false, fmt.Sprintf("类型不匹配: 期望 %s，实际 %s", expected.Type, actual.Type)
	}

	if expectedStr != actual.Str {
		return false, fmt.Sprintf("值不匹配: 期望 '%s'，实际 '%s'", expectedStr, actual.Str)
	}

	return true, ""
}

func StreamAssert(aggregatedContent string, chunkCount int, asserts []StreamAssertConfig) (bool, string) {
	logger.Debug("流式断言开始",
		zap.Int("块数", chunkCount),
		zap.Int("断言数", len(asserts)))

	for i, sa := range asserts {
		logger.Debug("执行流式断言",
			zap.Int("索引", i),
			zap.String("类型", sa.Kind),
			zap.String("模式", sa.Pattern),
			zap.Int("最小块数", sa.MinChunks))

		if ok, _ := checkStreamAssert(aggregatedContent, chunkCount, sa); ok {
			logger.Debug("流式断言通过", zap.Int("索引", i))
			return true, ""
		}
	}
	logger.Debug("所有流式断言均未通过")
	return false, "无可匹配的流式断言"
}

type StreamAssertConfig struct {
	Kind      string `json:"kind"`
	Pattern   string `json:"pattern"`
	MaxWaitMs int    `json:"max_wait_ms"`
	MinChunks int    `json:"min_chunks"`
}

func checkStreamAssert(aggregatedContent string, chunkCount int, sa StreamAssertConfig) (bool, string) {
	if chunkCount < sa.MinChunks {
		return false, fmt.Sprintf("需要至少 %d 个数据块，实际 %d 个", sa.MinChunks, chunkCount)
	}

	switch sa.Kind {
	case "contains":
		if strings.Contains(aggregatedContent, sa.Pattern) {
			return true, ""
		}
		return false, fmt.Sprintf("聚合内容不包含 '%s'", sa.Pattern)

	case "regex":
		matched, err := regexp.MatchString(sa.Pattern, aggregatedContent)
		if err != nil {
			return false, fmt.Sprintf("无效的正则表达式: %s", err.Error())
		}
		if matched {
			return true, ""
		}
		return false, fmt.Sprintf("聚合内容不匹配正则 '%s'", sa.Pattern)

	case "json_path":
		result := gjson.Get(aggregatedContent, sa.Pattern)
		if result.Exists() {
			return true, ""
		}
		return false, fmt.Sprintf("聚合内容中未找到 JSON 路径 '%s'", sa.Pattern)

	default:
		return false, fmt.Sprintf("未知的流式断言类型: %s", sa.Kind)
	}
}

func ExtractVariables(responseBody string, extractExpr string) (map[string]string, error) {
	if extractExpr == "" {
		return nil, nil
	}

	result := make(map[string]string)
	parts := strings.Split(extractExpr, ",")

	logger.Info("开始提取变量", zap.String("提取表达式", extractExpr), zap.String("响应体", responseBody))

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			logger.Warn("变量提取表达式格式错误，跳过", zap.String("片段", part))
			continue
		}

		key := strings.TrimSpace(kv[0])
		path := strings.TrimSpace(kv[1])
		// 兼容 JSONPath 风格的 $. 前缀；gjson 标准路径不需要 $
		path = strings.TrimPrefix(path, "$.")

		value := gjson.Get(responseBody, path)
		if value.Exists() {
			result[key] = value.String()
			logger.Info("变量提取成功", zap.String("变量名", key), zap.String("路径", path), zap.String("变量值", maskValue(value.String())))
		} else {
			logger.Warn("变量提取失败，路径不存在", zap.String("变量名", key), zap.String("路径", path))
		}
	}

	logger.Info("变量提取完成", zap.Any("结果", result))
	return result, nil
}

// maskValue 对长度较长的值做掩码，避免日志泄露完整 token
func maskValue(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "***" + s[len(s)-6:]
}

// truncateForLog 截断长字符串用于日志输出
func truncateForLog(s string) string {
	if len(s) <= 200 {
		return s
	}
	return s[:100] + "...(截断，共 " + fmt.Sprint(len(s)) + " 字符)"
}

func BuildAggregatedResult(aggregatedContent string, chunkCount int) string {
	result := map[string]interface{}{
		"aggregated_content": aggregatedContent,
		"chunk_count":        chunkCount,
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// fixRegexEscapes 修复正则表达式中丢失的反斜杠
// 当用户在 PSV 文件中写 {{regex:\d+}} 时，经过 JSON 解析后 \d 会变成 d
// 此函数自动检测并修复常见的正则表达式转义序列
func fixRegexEscapes(pattern string) string {
	if pattern == "" {
		return pattern
	}

	var result strings.Builder
	i := 0

	for i < len(pattern) {
		c := pattern[i]

		// 如果当前字符是反斜杠，直接保留并跳过下一个字符
		if c == '\\' && i+1 < len(pattern) {
			result.WriteByte(c)
			result.WriteByte(pattern[i+1])
			i += 2
			continue
		}

		// 检查是否是需要转义的字符（正则表达式中的特殊字符）
		switch c {
		case 'd', 'D', 'w', 'W', 's', 'S', 'b', 'B', 'n', 't', 'r':
			result.WriteByte('\\')
		}

		result.WriteByte(c)
		i++
	}

	return result.String()
}

// validateRegexPattern 验证正则表达式模式是否有效
// 如果模式以重复操作符开头，自动添加 . 作为前缀
func validateRegexPattern(pattern string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("模式为空")
	}

	// 检查模式是否以重复操作符开头
	firstChar := pattern[0]
	if firstChar == '*' || firstChar == '+' || firstChar == '?' || firstChar == '{' {
		// 如果以重复操作符开头，在前面添加 . 匹配任意字符
		pattern = "." + pattern
	}

	// 检查模式中是否有孤立的重复操作符（前面没有有效字符）
	for i := 1; i < len(pattern); i++ {
		c := pattern[i]
		if c == '*' || c == '+' || c == '?' {
			prevChar := pattern[i-1]
			// 如果前一个字符也是特殊字符，需要处理
			if prevChar == '(' || prevChar == '[' || prevChar == '|' || prevChar == '^' || prevChar == '$' {
				// 在重复操作符前插入 .
				pattern = pattern[:i] + "." + pattern[i:]
				i++ // 跳过新插入的字符
			}
		}
	}

	return pattern, nil
}