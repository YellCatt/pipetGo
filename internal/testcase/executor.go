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

	"pipet/config"
	"pipet/internal/assert"
	"pipet/internal/httpclient"
	"pipet/internal/logger"
	"pipet/internal/psv"
	"pipet/internal/vars"
)

type TestResult struct {
	TestCase     psv.TestCase
	Passed       bool
	Error        string
	Duration     time.Duration
	StartTime    time.Time
	EndTime      time.Time
	ResponseBody string
}

var (
	results      []TestResult
	resultsMu    sync.Mutex
	globalVars   = make(map[string]string)
	globalVarsMu sync.Mutex
)

func ExecuteTestCase(tc psv.TestCase) TestResult {
	startTime := time.Now()
	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	logger.Info("Running test", zap.String("id", tc.ID), zap.String("desc", tc.Desc))

	if tc.Skip {
		logger.Info("Skipping test", zap.String("id", tc.ID))
		result.Passed = true
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result
	}

	processedURL := vars.Replace(tc.URL)
	processedHeaders := make(map[string]string)
	for k, v := range tc.Headers {
		processedHeaders[k] = vars.Replace(v)
	}
	processedBody := vars.Replace(tc.Body)
	processedJSON := vars.Replace(tc.JSON)

	req := httpclient.Client.R()
	for k, v := range processedHeaders {
		req.SetHeader(k, v)
	}

	for k, v := range tc.Params {
		req.SetQueryParam(k, vars.Replace(v))
	}

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
		req.SetHeader("Content-Type", "application/json")
		req.SetBody(processedJSON)
	} else if len(tc.Form) > 0 {
		formData := make(map[string]string)
		for k, v := range tc.Form {
			formData[k] = vars.Replace(v)
		}
		req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
		req.SetFormData(formData)
	} else if tc.Body != "" {
		req.SetBody(processedBody)
	} else if tc.Payload != "" {
		req.SetBody(vars.Replace(tc.Payload))
	}

	var resp *resty.Response
	var err error

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

	if err != nil {
		result.Error = err.Error()
		result.Passed = false
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		logger.Error("Test failed", zap.String("id", tc.ID), zap.Error(err))
		return result
	}

	result.ResponseBody = string(resp.Body())

	if tc.StreamMode {
		result = executeStreamAssert(tc, resp, startTime)
	} else {
		if tc.ExpectedStatus > 0 && resp.StatusCode() != tc.ExpectedStatus {
			result.Error = fmt.Sprintf("expected status %d, got %d", tc.ExpectedStatus, resp.StatusCode())
			result.Passed = false
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
			return result
		}

		if tc.BodyRegex != "" {
			if ok, errMsg := assert.BodyRegexMatch(result.ResponseBody, tc.BodyRegex); !ok {
				result.Error = errMsg
				result.Passed = false
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
				return result
			}
		}

		if tc.ExpectedBody != "" {
			if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), result.ResponseBody, tc.MatchMode); !ok {
				result.Error = errMsg
				result.Passed = false
				result.EndTime = time.Now()
				result.Duration = result.EndTime.Sub(startTime)
				logger.Error("Test failed", zap.String("id", tc.ID), zap.String("error", result.Error))
				return result
			}
		}
	}

	if tc.Extract != "" {
		extractedVars, err := assert.ExtractVariables(result.ResponseBody, tc.Extract)
		if err == nil {
			globalVarsMu.Lock()
			for k, v := range extractedVars {
				globalVars[k] = v
				vars.Set(k, v)
			}
			globalVarsMu.Unlock()
			logger.Info("Extracted variables", zap.String("id", tc.ID), zap.Any("vars", extractedVars))
		}
	}

	result.Passed = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	logger.Info("Test passed", zap.String("id", tc.ID), zap.Duration("duration", result.Duration))

	return result
}

func hasFileField(form map[string]string) bool {
	for _, v := range form {
		if strings.HasPrefix(v, "@") || strings.HasPrefix(v, "file://") {
			return true
		}
	}
	return false
}

func executeStreamAssert(tc psv.TestCase, resp *resty.Response, startTime time.Time) TestResult {
	result := TestResult{
		TestCase:  tc,
		StartTime: startTime,
	}

	body := string(resp.Body())
	scanner := bufio.NewScanner(strings.NewReader(body))

	var aggregatedContent strings.Builder
	chunkCount := 0

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

		if ok, _ := assert.StreamAssert(aggregatedContent.String(), chunkCount, assertConfigs); ok {
			result.Passed = true
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			logger.Info("Stream assertion passed", zap.String("id", tc.ID))
			return result
		}
	}

	aggregatedResult := assert.BuildAggregatedResult(aggregatedContent.String(), chunkCount)
	result.ResponseBody = aggregatedResult

	if tc.ExpectedBody != "" {
		if ok, errMsg := assert.JSONMatch(vars.Replace(tc.ExpectedBody), aggregatedResult, tc.MatchMode); !ok {
			result.Error = errMsg
			result.Passed = false
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(startTime)
			return result
		}
	}

	result.Passed = true
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result
}

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

func RunParallel(testCases []psv.TestCase) []TestResult {
	var wg sync.WaitGroup
	resultChan := make(chan TestResult, len(testCases))

	for _, tc := range testCases {
		wg.Add(1)
		go func(tc psv.TestCase) {
			defer wg.Done()
			result := ExecuteTestCase(tc)
			resultChan <- result
		}(tc)
	}

	wg.Wait()
	close(resultChan)

	var results []TestResult
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

func GetResults() []TestResult {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	return append([]TestResult{}, results...)
}

func AddResult(result TestResult) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, result)
}

func GenerateReport(results []TestResult) (string, string) {
	var allReport strings.Builder
	var errorReport strings.Builder

	allReport.WriteString("id|skip|desc|method|url|expected_status|actual_status|passed|error|duration|start_time|end_time\n")
	errorReport.WriteString("id|skip|desc|method|url|expected_status|actual_status|passed|error|duration|start_time|end_time|response_body\n")

	for _, result := range results {
		line := fmt.Sprintf("%s|%t|%s|%s|%s|%d|%d|%t|%s|%s|%s|%s\n",
			result.TestCase.ID,
			result.TestCase.Skip,
			result.TestCase.Desc,
			result.TestCase.Method,
			result.TestCase.URL,
			result.TestCase.ExpectedStatus,
			0,
			result.Passed,
			result.Error,
			result.Duration,
			result.StartTime.Format(time.RFC3339),
			result.EndTime.Format(time.RFC3339),
		)
		allReport.WriteString(line)

		if !result.Passed {
			errorLine := line[:len(line)-1] + "|" + escapePipe(result.ResponseBody) + "\n"
			errorReport.WriteString(errorLine)
		}
	}

	return allReport.String(), errorReport.String()
}

func escapePipe(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func SaveReports(allReport, errorReport string) (string, string) {
	timestamp := time.Now().Format("20060102_150405")
	reportDir := config.AppConfig.Test.ReportDir

	if err := os.MkdirAll(reportDir, 0755); err != nil {
		logger.Error("Failed to create report directory", zap.Error(err))
		return "", ""
	}

	allPath := fmt.Sprintf("%s/report_%s.psv", reportDir, timestamp)
	if err := os.WriteFile(allPath, []byte(allReport), 0644); err != nil {
		logger.Error("Failed to save report", zap.Error(err))
	}

	var errorPath string
	if errorReport != "" {
		errorPath = fmt.Sprintf("%s/report_%s_error.psv", reportDir, timestamp)
		if err := os.WriteFile(errorPath, []byte(errorReport), 0644); err != nil {
			logger.Error("Failed to save error report", zap.Error(err))
		}
	}

	logger.Info("Reports saved", zap.String("all", allPath), zap.String("error", errorPath))
	return allPath, errorPath
}

func PrintSummary(results []TestResult) {
	var passed, failed, skipped int
	var totalDuration time.Duration

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

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║              pipet 接口测试                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	for _, r := range results {
		status := "PASS"
		if r.TestCase.Skip {
			status = "SKIP"
		} else if !r.Passed {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %s ... %s (%.3fs)\n", r.TestCase.ID, r.TestCase.Desc, status, r.Duration.Seconds())
		if !r.Passed && !r.TestCase.Skip {
			fmt.Printf("       Error: %s\n", r.Error)
		}
	}

	fmt.Println()
	fmt.Printf("Total: %d, Passed: %d, Failed: %d, Skipped: %d\n", passed+failed+skipped, passed, failed, skipped)
	fmt.Printf("Total duration: %.3fs\n", totalDuration.Seconds())
}