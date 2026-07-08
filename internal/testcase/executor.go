// Package testcase 提供测试用例执行和结果管理功能
// 包括 HTTP 请求执行、断言验证、变量提取和报告生成
package testcase

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"pipetGo/config"
	"pipetGo/internal/assert"
	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
	"pipetGo/internal/psv"
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
	"pipetGo/internal/vars"
)


// TestResult 表示测试用例执行结果
type TestResult struct {
	TestCase       psv.TestCase  // 测试用例
	Passed         bool          // 是否通过
	Error          string        // 错误信息
	Duration       time.Duration // 执行时长
	StartTime      time.Time     // 开始时间
	EndTime        time.Time     // 结束时间
	ResponseBody   string        // 响应体
	ActualStatus   int           // 实际状态码
	RequestHeaders string        // 请求头（JSON格式）
	RequestBody    string        // 请求体
	ExtractedVars  string        // 提取的变量（JSON格式）
}

// 全局变量用于存储测试结果和提取的变量
var (
	results      []TestResult              // 测试结果列表
	resultsMu    sync.Mutex                // 保护 results 的互斥锁
	globalVars   = make(map[string]string) // 全局变量存储
	globalVarsMu sync.Mutex                // 保护 globalVars 的互斥锁
)

// ExecuteTestCase 执行单个测试用例
// tc: 测试用例
// 返回: 测试结果
func ExecuteTestCase(tc psv.TestCase) TestResult {
	startTime := timeutil.Now()

	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	logger.Info("Running test", zap.String("id", tc.ID), zap.String("desc", tc.Desc))

	// 如果标记为跳过，直接返回通过
	if tc.Skip {
		logger.Info("Skipping test", zap.String("id", tc.ID))
		result.Passed = true
		result.EndTime = timeutil.Now()
		result.Duration = result.EndTime.Sub(startTime)
		fmt.Printf("[%s] [%s] %s ... SKIP (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		return result
	}


	// 变量替换：将 {{var}} 替换为实际值
	processedURL := vars.Replace(tc.URL)
	processedHeaders := make(map[string]string)
	for k, v := range tc.Headers {
		processedHeaders[k] = vars.Replace(v)
	}
	processedBody := vars.Replace(tc.Body)
	processedJSON := vars.Replace(tc.JSON)

	// 构建请求体（用于报告记录）
	var requestBody string
	if tc.JSON != "" {
		requestBody = processedJSON
	} else if tc.Body != "" {
		requestBody = processedBody
	} else if tc.Payload != "" {
		requestBody = vars.Replace(tc.Payload)
	} else if len(tc.Form) > 0 {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		formJSON, _ := json.Marshal(formData)
		requestBody = string(formJSON)
	}

	// 请求头转JSON（用于报告记录）
	headersJSON, _ := json.Marshal(processedHeaders)
	result.RequestHeaders = string(headersJSON)
	result.RequestBody = requestBody

	// 创建 HTTP 请求
	req := httpclient.Client.R()
	for k, v := range processedHeaders {
		req.SetHeader(k, v)
	}

	// 设置 URL 参数
	for k, v := range tc.Params {
		req.SetQueryParam(k, vars.Replace(v))
	}

	// 处理表单数据（支持文件上传）
	if hasFileField(tc.Form) {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			v = vars.Replace(v)
			if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
				filePath := strings.TrimPrefix(strings.TrimPrefix(v, "@"), "file://")
				req.SetFile(k, filePath)
			} else {
				formData[k] = v
			}
		}
		if len(formData) > 0 {
			req.SetFormData(formData)
		}
	} else if tc.JSON != "" {
		// JSON 请求体
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(processedJSON)
	} else if len(tc.Form) > 0 {
		// 表单数据
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
		req.SetFormData(formData)
	} else if tc.Body != "" {
		// 原始请求体
		req.SetBody(processedBody)
	} else if tc.Payload != "" {
		// 兼容性字段
		req.SetBody(vars.Replace(tc.Payload))
	}

	var resp *resty.Response
	var err error

	// 根据 HTTP 方法执行请求
	switch tc.Method {
	case http.MethodGet:
		resp, err = req.Get(processedURL)
	case http.MethodPost:
		resp, err = req.Post(processedURL)
	case http.MethodPut:
		resp, err = req.Put(processedURL)
	case http.MethodDelete:
		resp, err = req.Delete(processedURL)
	case http.MethodPatch:
		resp, err = req.Patch(processedURL)
	case http.MethodHead:
		resp, err = req.Head(processedURL)
	default:
		err = fmt.Errorf("unsupported HTTP method: %s", tc.Method)
	}

	// 请求执行失败
	if err != nil {
		result.Error = err.Error()
		result.Passed = false
		result.EndTime = timeutil.Now()
		result.Duration = result.EndTime.Sub(startTime)
		logger.Error("Test failed", zap.String("id", tc.ID), zap.Error(err))
		fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
		fmt.Printf("            Error: %s\n", result.Error)
		return result
	}


	result.ResponseBody = string(resp.Body())
	result.ActualStatus = resp.StatusCode()

	// 流式模式处理
	if tc.StreamMode {
		result = executeStreamAssert(tc, resp, startTime)
	} else {
		// 普通模式断言
		if tc.ExpectedStatus > 0 && resp.StatusCode() != tc.ExpectedStatus {
			result.Error = fmt.Sprintf("expected status %d, got %d", tc.ExpectedStatus, resp.StatusCode())
			result.Passed = false
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
			fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			fmt.Printf("            Error: %s\n", result.Error)
			return result
		}


		// 正则表达式断言
		if tc.BodyRegex != "" {
			if ok, errMsg := assert.BodyRegexMatch(result.ResponseBody, tc.BodyRegex); !ok {
				result.Error = errMsg
				result.Passed = false
				result.EndTime = timeutil.Now()
				result.Duration = result.EndTime.Sub(startTime)
				logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
				fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
				fmt.Printf("            Error: %s\n", result.Error)
				return result
			}
		}


		// JSON 响应体断言
		if tc.ExpectedBody != "" {
			if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), result.ResponseBody, tc.MatchMode); !ok {
				result.Error = errMsg
				result.Passed = false
				result.EndTime = timeutil.Now()
				result.Duration = result.EndTime.Sub(startTime)
				logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
				fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
				fmt.Printf("            Error: %s\n", result.Error)
				return result
			}
		}

	}

	// 提取变量（用于链式测试）
	if tc.Extract != "" {
		extractedVars, err := assert.ExtractVariables(result.ResponseBody, tc.Extract)
		if err == nil {
			globalVarsMu.Lock()
			for k, v := range extractedVars {
				globalVars[k] = v
				vars.Set(k, v)
			}
			globalVarsMu.Unlock()
			// 记录提取的变量到结果中
			extractedVarsJSON, _ := json.Marshal(extractedVars)
			result.ExtractedVars = string(extractedVarsJSON)
			logger.Info("Extracted variables", zap.String("id", tc.ID), zap.Any("vars", extractedVars))
		}
	}

	// 测试通过
	result.Passed = true
	result.EndTime = timeutil.Now()
	result.Duration = result.EndTime.Sub(startTime)
	logger.Info("Test passed", zap.String("id", tc.ID), zap.Duration("duration", result.Duration))

	// 打印测试结果
	fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())

	// 记录成功的执行时间到数据库
	go storage.RecordExecutionTime(tc.ID, tc.Desc, vars.Replace(tc.URL), result.Duration, true)

	return result
}


// hasFileField 检查表单是否包含文件上传字段
// form: 表单数据
// 返回: 是否包含文件字段
func hasFileField(form map[string]string) bool {
	for _, v := range form {
		if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
			return true
		}
	}
	return false
}

// executeStreamAssert 执行流式响应断言
// tc: 测试用例
// resp: HTTP 响应
// startTime: 开始时间
// 返回: 测试结果
func executeStreamAssert(tc psv.TestCase, resp *resty.Response, startTime time.Time) TestResult {
	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	body := string(resp.Body())
	scanner := bufio.NewScanner(strings.NewReader(body))

	var aggregatedContent strings.Builder
	chunkCount := 0

	// 解析 SSE（Server-Sent Events）格式响应
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			chunkCount++

			var jsonData map[string]interface{}
			if json.Unmarshal([]byte(data), &jsonData) == nil {
				if choices, ok := jsonData["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								aggregatedContent.WriteString(content)
							}
						}
					}
				}
			}
		}
	}

	// 执行流式断言
	if len(tc.StreamAssert) > 0 {
		assertConfigs := make([]assert.StreamAssertConfig, len(tc.StreamAssert))
		for i, sa := range tc.StreamAssert {
			assertConfigs[i] = assert.StreamAssertConfig{
				Kind:      sa.Kind,
				Pattern:   vars.Replace(sa.Pattern),
				MaxWaitMs: sa.MaxWaitMs,
				MinChunks: sa.MinChunks,
			}
		}

		if ok, errMsg := assert.StreamAssert(aggregatedContent.String(), chunkCount, assertConfigs); ok {
			result.Passed = true
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			logger.Info("Stream assertion passed", zap.String("id", tc.ID))
			fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			return result
		} else {
			result.Error = errMsg
			result.Passed = false
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			fmt.Printf("            Error: %s\n", result.Error)
			return result
		}

	}

	// 构建聚合结果
	aggregatedResult := assert.BuildAggregatedResult(aggregatedContent.String(), chunkCount)
	result.ResponseBody = aggregatedResult

	// JSON 响应体断言
	if tc.ExpectedBody != "" {
		if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), aggregatedResult, tc.MatchMode); !ok {
			result.Error = errMsg
			result.Passed = false
			result.EndTime = timeutil.Now()
			result.Duration = result.EndTime.Sub(startTime)
			fmt.Printf("[%s] [%s] %s ... FAIL (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
			fmt.Printf("            Error: %s\n", result.Error)
			return result
		}
	}

	result.Passed = true
	result.EndTime = timeutil.Now()
	result.Duration = result.EndTime.Sub(startTime)
	fmt.Printf("[%s] [%s] %s ... PASS (%.3fs)\n", timeutil.FormatDateTime(result.EndTime), tc.ID, tc.Desc, result.Duration.Seconds())
	return result
}


// FilterByTags 根据标签过滤测试用例
// testCases: 测试用例列表
// tags: 标签列表
// 返回: 过滤后的测试用例列表
func FilterByTags(testCases []psv.TestCase, tags []string) []psv.TestCase {
	if len(tags) == 0 {
		return testCases
	}

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[strings.ToLower(tag)] = true
	}

	var filtered []psv.TestCase
	for _, tc := range testCases {
		for _, tag := range tc.Tags {
			if tagSet[strings.ToLower(tag)] {
				filtered = append(filtered, tc)
				break
			}
		}
	}

	return filtered
}

// RunParallel 执行测试用例（串行模式，适用于资源受限设备）
// testCases: 测试用例列表
// 返回: 测试结果列表
func RunParallel(testCases []psv.TestCase) []TestResult {
	var results []TestResult

	for _, tc := range testCases {
		result := ExecuteTestCase(tc)
		results = append(results, result)
	}

	return results
}

// GetResults 获取所有测试结果
// 返回: 测试结果列表
func GetResults() []TestResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]TestResult{}, results...)
}

// AddResult 添加测试结果
// result: 测试结果
func AddResult(result TestResult) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, result)
}

// GenerateReport 生成测试报告
// results: 测试结果列表
// 返回: 全部报告内容, 错误报告内容
func GenerateReport(results []TestResult) (string, string) {
	var allReport strings.Builder
	var errorReport strings.Builder

	// 使用新的报告格式（添加东八区时间字段）
	header := "id|desc|method|url|request_headers|request_body|tags|status|duration_s|expect_status|actual_status|diff|actual_body|expect_body|pre_conditions|post_conditions|extracted_vars|start_time|end_time\n"
	allReport.WriteString(header)
	errorReport.WriteString(header)

	for _, result := range results {
		// 确定测试状态
		status := "PASS"
		if result.TestCase.Skip {
			status = "SKIP"
		} else if !result.Passed {
			status = "FAIL"
		}

		// 标签列表转字符串
		tags := strings.Join(result.TestCase.Tags, ",")

		// 前置条件转字符串
		preConditions := strings.Join(result.TestCase.Pre, ";")

		// 后置条件转字符串
		postConditions := strings.Join(result.TestCase.Post, ";")

		// 差异信息（错误信息）
		diff := result.Error

		processedURL := vars.Replace(result.TestCase.URL)
		processedExpectedBody := vars.Replace(result.TestCase.ExpectedBody)

	// 格式化东八区时间
	startTime := timeutil.FormatDateTime(result.StartTime)
	endTime := timeutil.FormatDateTime(result.EndTime)


		line := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%.3f|%d|%d|%s|%s|%s|%s|%s|%s|%s|%s\n",
			escapePipe(result.TestCase.ID),
			escapePipe(result.TestCase.Desc),
			result.TestCase.Method,
			escapePipe(processedURL),
			escapePipe(result.RequestHeaders),
			escapePipe(result.RequestBody),
			tags,
			status,
			result.Duration.Seconds(),
			result.TestCase.ExpectedStatus,
			result.ActualStatus,
			escapePipe(diff),
			escapePipe(result.ResponseBody),
			escapePipe(processedExpectedBody),
			escapePipe(preConditions),
			escapePipe(postConditions),
			escapePipe(result.ExtractedVars),
			startTime,
			endTime,
		)
		allReport.WriteString(line)

		// 错误报告只包含失败的测试用例
		if !result.Passed && !result.TestCase.Skip {
			errorReport.WriteString(line)
		}
	}

	return allReport.String(), errorReport.String()
}

// escapePipe 转义管道符
// s: 输入字符串
// 返回: 转义后的字符串
func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// SaveReports 保存测试报告到文件

// allReport: 全部报告内容
// errorReport: 错误报告内容
// 返回: 全部报告路径, 错误报告路径
func SaveReports(allReport, errorReport string, timestamp ...string) (string, string) {
	// 如果没有提供时间戳，使用当前时间
	ts := timeutil.FormatCompact(timeutil.Now())
	if len(timestamp) > 0 && timestamp[0] != "" {
		ts = timestamp[0]
	}
	
	reportDir := config.AppConfig.Test.ReportDir


	// 创建报告目录
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", ""
	}

	// 保存全部报告
	allPath := fmt.Sprintf("%s/report_%s.psv", reportDir, ts)
	if err := os.WriteFile(allPath, []byte(allReport), 0644); err != nil {
		logger.Error("Failed to save report", zap.Error(err))
	}

	// 保存错误报告（如果有失败的测试）
	var errorPath string
	if errorReport != "" {
		errorPath = fmt.Sprintf("%s/report_%s_error.psv", reportDir, ts)
		if err := os.WriteFile(errorPath, []byte(errorReport), 0644); err != nil {
			logger.Error("Failed to save error report", zap.Error(err))
		}
	}

	logger.Info("Reports saved", zap.String("all", allPath), zap.String("error", errorPath))
	return allPath, errorPath
}

// PrintSummary 打印测试摘要
// results: 测试结果列表
func PrintSummary(results []TestResult) {
	var passed, failed, skipped int
	var totalDuration time.Duration

	// 统计结果
	for _, r := range results {
		totalDuration += r.Duration
		if r.TestCase.Skip {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	// 打印汇总信息
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║              pipet 接口测试                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Total: %d, Passed: %d, Failed: %d, Skipped: %d\n", passed+failed+skipped, passed, failed, skipped)
	fmt.Printf("Total duration: %.3fs\n", totalDuration.Seconds())
}