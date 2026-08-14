// Package storage 提供基于 CSV 文件的测试执行数据持久化
// 替代原来的 SQLite 数据库，每张表对应一个 CSV 文件
package storage

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"pipetGo/internal/logger"
	"pipetGo/internal/timeutil"
)

var (
	dataDir string
	mu      sync.RWMutex
	once    sync.Once
)

var (
	executionHeader = []string{
		"test_case_id",
		"test_case_desc",
		"file_name",
		"url",
		"duration_ms",
		"success",
		"executed_at",
	}

	averageHeader = []string{
		"test_case_id",
		"test_case_desc",
		"file_name",
		"url",
		"average_duration_ms",
		"execution_count",
		"last_updated",
	}

	dailyHeader = []string{
		"date",
		"total",
		"passed",
		"failed",
		"skipped",
		"error_rate",
		"total_duration_ms",
		"unique_cases",
	}
)

// InitDB 初始化 CSV 数据目录（单例模式，保持与原 SQLite 接口一致）
func InitDB(dir string) error {
	var initErr error
	once.Do(func() {
		initErr = initCSVInternal(dir)
	})
	return initErr
}

func initCSVInternal(dir string) error {
	logger.Info("========== 开始初始化 CSV 存储 ==========")
	logger.Info("数据目录参数值", zap.String("数据目录", dir))

	if dir == "" {
		logger.Info("数据目录为空，使用默认值 ./sql")
		dir = "./sql"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Error("创建数据目录失败", zap.String("数据目录", dir), zap.Error(err))
		return err
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		logger.Error("获取数据目录绝对路径失败", zap.String("目录", dir), zap.Error(err))
		return err
	}
	dataDir = absDir
	logger.Info("数据目录创建成功", zap.String("数据目录", dataDir))

	if err := ensureCSV(executionCSVPath(), executionHeader); err != nil {
		logger.Error("初始化执行记录 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(averageCSVPath(), averageHeader); err != nil {
		logger.Error("初始化平均时间 CSV 失败", zap.Error(err))
		return err
	}
	if err := ensureCSV(dailyCSVPath(), dailyHeader); err != nil {
		logger.Error("初始化每日汇总 CSV 失败", zap.Error(err))
		return err
	}

	logger.Info("CSV 存储初始化成功",
		zap.String("executionCSV", executionCSVPath()),
		zap.String("averageCSV", averageCSVPath()),
		zap.String("dailyCSV", dailyCSVPath()))
	return nil
}

func executionCSVPath() string {
	return filepath.Join(dataDir, "test_execution_times.csv")
}

func averageCSVPath() string {
	return filepath.Join(dataDir, "test_average_times.csv")
}

func dailyCSVPath() string {
	return filepath.Join(dataDir, "test_daily_summary.csv")
}

// ensureCSV 如果 CSV 文件不存在或为空，则创建并写入表头
func ensureCSV(path string, header []string) error {
	info, err := os.Stat(path)
	if err == nil && info.Size() > 0 {
		return nil
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(header); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// readRecords 读取 CSV 文件，返回表头和数据行
func readRecords(path string) ([]string, [][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(all) == 0 {
		return nil, nil, nil
	}
	return all[0], all[1:], nil
}

// appendRecord 向 CSV 文件追加一行记录
func appendRecord(path string, record []string) error {
	if err := ensureCSV(path, executionHeader); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(record); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// writeRecords 覆盖写入 CSV 文件（包含表头）
func writeRecords(path string, header []string, records [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	w := csv.NewWriter(file)
	if err := w.Write(header); err != nil {
		return err
	}
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func parseSuccess(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1"
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func parseFloat64(s string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return n
}

// RecordExecutionTime 记录测试执行时间
func RecordExecutionTime(testCaseID, testCaseDesc, fileName, url string, duration time.Duration, success bool) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	record := []string{
		testCaseID,
		testCaseDesc,
		fileName,
		url,
		strconv.FormatInt(int64(duration/time.Millisecond), 10),
		strconv.FormatBool(success),
		timeutil.Now().Format("2006-01-02 15:04:05"),
	}

	if err := appendRecord(executionCSVPath(), record); err != nil {
		logger.Error("记录执行时间失败", zap.Error(err))
		return err
	}
	return nil
}

// GetAverageDuration 获取指定 URL 的平均执行时间
func GetAverageDuration(url string) (time.Duration, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	var sum int64
	var count int64
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if rec[3] != url {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		sum += parseInt64(rec[4])
		count++
	}

	if count == 0 {
		return 0, nil
	}
	return time.Duration(sum/count) * time.Millisecond, nil
}

// GetAllAverageDurations 获取所有 URL 的平均执行时间
func GetAllAverageDurations() (map[string]time.Duration, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		logger.Warn("获取平均耗时: 存储未初始化")
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		logger.Warn("获取平均耗时: 读取失败", zap.Error(err))
		return nil, err
	}

	type agg struct {
		sum   int64
		count int64
	}
	groups := make(map[string]*agg)

	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		url := rec[3]
		if groups[url] == nil {
			groups[url] = &agg{}
		}
		groups[url].sum += parseInt64(rec[4])
		groups[url].count++
	}

	averages := make(map[string]time.Duration)
	for url, g := range groups {
		if g.count > 0 {
			averages[url] = time.Duration(g.sum/g.count) * time.Millisecond
		}
	}

	logger.Info("获取平均耗时: 已找到", zap.Int("数量", len(averages)), zap.Any("平均值列表", averages))
	if len(averages) == 0 {
		logger.Warn("获取平均耗时: 未找到历史数据")
	}
	return averages, nil
}

// GetExecutionCount 获取指定 URL 的成功执行次数
func GetExecutionCount(url string) (int, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	count := 0
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if rec[3] == url && parseSuccess(rec[5]) {
			count++
		}
	}
	return count, nil
}

// GetTotalExecutionCount 获取成功执行的总记录数
func GetTotalExecutionCount() (int, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return 0, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}

	count := 0
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if parseSuccess(rec[5]) {
			count++
		}
	}
	return count, nil
}

// CalculateAndStoreAverages 计算所有成功测试用例的平均执行时间并存储到 CSV
func CalculateAndStoreAverages() error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return err
	}

	type agg struct {
		testCaseID   string
		testCaseDesc string
		fileName     string
		url          string
		sum          int64
		count        int64
	}
	groups := make(map[string]*agg)

	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		if !parseSuccess(rec[5]) {
			continue
		}
		testCaseID := rec[0]
		fileName := rec[2]
		url := rec[3]
		key := testCaseID + "\x00" + fileName + "\x00" + url
		if groups[key] == nil {
			var desc string
			if len(rec) > 1 {
				desc = rec[1]
			}
			groups[key] = &agg{
				testCaseID:   testCaseID,
				testCaseDesc: desc,
				fileName:     fileName,
				url:          url,
			}
		}
		groups[key].sum += parseInt64(rec[4])
		groups[key].count++
	}

	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	now := timeutil.Now().Format("2006-01-02 15:04:05")
	var avgRecords [][]string
	for _, k := range keys {
		g := groups[k]
		if g.count == 0 {
			continue
		}
		avg := float64(g.sum) / float64(g.count)
		avgRecords = append(avgRecords, []string{
			g.testCaseID,
			g.testCaseDesc,
			g.fileName,
			g.url,
			strconv.FormatFloat(avg, 'f', -1, 64),
			strconv.FormatInt(g.count, 10),
			now,
		})
		logger.Info("已存储平均耗时",
			zap.String("test_case_id", g.testCaseID),
			zap.String("file_name", g.fileName),
			zap.String("url", g.url),
			zap.Float64("avg_ms", avg),
			zap.Int64("count", g.count))
	}

	if err := writeRecords(averageCSVPath(), averageHeader, avgRecords); err != nil {
		logger.Error("存储平均耗时失败", zap.Error(err))
		return err
	}

	return nil
}

// GetAllStoredAverages 获取所有已存储的平均执行时间
func GetAllStoredAverages() ([]map[string]interface{}, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	header, records, err := readRecords(averageCSVPath())
	if err != nil {
		return nil, err
	}

	colIndex := make(map[string]int)
	for i, h := range header {
		colIndex[strings.TrimSpace(h)] = i
	}

	get := func(rec []string, name string) string {
		if idx, ok := colIndex[name]; ok && idx < len(rec) {
			return rec[idx]
		}
		return ""
	}

	var averages []map[string]interface{}
	for _, rec := range records {
		averages = append(averages, map[string]interface{}{
			"test_case_id":        get(rec, "test_case_id"),
			"test_case_desc":      get(rec, "test_case_desc"),
			"file_name":           get(rec, "file_name"),
			"url":                 get(rec, "url"),
			"average_duration_ms": parseFloat64(get(rec, "average_duration_ms")),
			"execution_count":     int(parseInt64(get(rec, "execution_count"))),
			"last_updated":        get(rec, "last_updated"),
		})
	}

	return averages, nil
}

// DailySummary 表示单日测试汇总
type DailySummary struct {
	Date            string
	Total           int
	Passed          int
	Failed          int
	Skipped         int
	ErrorRate       float64
	TotalDurationMs int64
	UniqueCases     int
}

// RecordDailySummary 记录单日汇总
func RecordDailySummary(date string, total, passed, failed, skipped int, totalDuration time.Duration) error {
	mu.Lock()
	defer mu.Unlock()

	if dataDir == "" {
		return fmt.Errorf("storage not initialized")
	}

	uniqueCases, _ := countUniqueCaseIDsOnDate(date)
	var errorRate float64
	if total > 0 {
		errorRate = float64(failed) / float64(total) * 100
	}

	record := []string{
		date,
		strconv.Itoa(total),
		strconv.Itoa(passed),
		strconv.Itoa(failed),
		strconv.Itoa(skipped),
		strconv.FormatFloat(errorRate, 'f', 2, 64),
		strconv.FormatInt(int64(totalDuration/time.Millisecond), 10),
		strconv.Itoa(uniqueCases),
	}

	return appendRecord(dailyCSVPath(), record)
}

func countUniqueCaseIDsOnDate(date string) (int, error) {
	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool)
	for _, rec := range records {
		if len(rec) < 7 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(rec[6]), date) {
			seen[rec[0]] = true
		}
	}
	return len(seen), nil
}

// GetDailySummaryFromExecutions 从执行记录实时计算指定日期的汇总
func GetDailySummaryFromExecutions(date string) (*DailySummary, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return nil, err
	}

	var total, passed, failed, skipped int
	var totalDurationMs int64
	uniqueCases := make(map[string]bool)

	for _, rec := range records {
		if len(rec) < 7 {
			continue
		}
		executedAt := strings.TrimSpace(rec[6])
		if !strings.HasPrefix(executedAt, date) {
			continue
		}

		uniqueCases[rec[0]] = true
		durationMs := parseInt64(rec[4])
		success := parseSuccess(rec[5])

		total++
		totalDurationMs += durationMs
		if success {
			passed++
		} else {
			failed++
		}
	}

	if total == 0 {
		return nil, nil
	}

	errorRate := 0.0
	if total > 0 {
		errorRate = float64(failed) / float64(total) * 100
	}

	summary := &DailySummary{
		Date:            date,
		Total:           total,
		Passed:          passed,
		Failed:          failed,
		Skipped:         skipped,
		ErrorRate:       errorRate,
		TotalDurationMs: totalDurationMs,
		UniqueCases:     len(uniqueCases),
	}
	return summary, nil
}

// GetDailySummaries 获取指定时间范围内的每日汇总
func GetDailySummaries(fromDate, toDate string) ([]DailySummary, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(dailyCSVPath())
	if err != nil {
		return nil, err
	}

	dateMap := make(map[string]DailySummary)
	for _, rec := range records {
		if len(rec) < 8 {
			continue
		}
		date := strings.TrimSpace(rec[0])
		if fromDate != "" && date < fromDate {
			continue
		}
		if toDate != "" && date > toDate {
			continue
		}
		dateMap[date] = DailySummary{
			Date:            date,
			Total:           int(parseInt64(rec[1])),
			Passed:          int(parseInt64(rec[2])),
			Failed:          int(parseInt64(rec[3])),
			Skipped:         int(parseInt64(rec[4])),
			ErrorRate:       parseFloat64(rec[5]),
			TotalDurationMs: parseInt64(rec[6]),
			UniqueCases:     int(parseInt64(rec[7])),
		}
	}

	var summaries []DailySummary
	for _, s := range dateMap {
		summaries = append(summaries, s)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Date < summaries[j].Date
	})
	// 调试日志：raw_records 为读取到的全部记录数，result 为按日期区间过滤后的结果数
	logger.Debug("查询每日汇总",
		zap.Int("原始记录数", len(records)),
		zap.String("起始日期", fromDate),
		zap.String("结束日期", toDate),
		zap.Int("结果条数", len(summaries)))
	return summaries, nil
}

// CaseAvgDuration 表示单个用例的平均耗时（用于排序展示）
type CaseAvgDuration struct {
	TestCaseID        string
	TestCaseDesc      string
	FileName          string
	URL               string
	AverageDurationMs float64
	ExecutionCount    int
}

// GetCaseAverageDurations 获取每个测试用例的平均耗时（含慢接口排名）
func GetCaseAverageDurations(order string) ([]CaseAvgDuration, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return nil, err
	}

	type agg struct {
		id   string
		desc string
		file string
		url  string
		sum  int64
		n    int
	}
	groups := make(map[string]*agg)

	for _, rec := range records {
		if len(rec) < 7 {
			continue
		}
		id := rec[0]
		g, ok := groups[id]
		if !ok {
			g = &agg{
				id:   id,
				desc: rec[1],
				file: rec[2],
				url:  rec[3],
			}
			groups[id] = g
		}
		g.sum += parseInt64(rec[4])
		g.n++
	}

	var result []CaseAvgDuration
	for _, g := range groups {
		if g.n == 0 {
			continue
		}
		avg := float64(g.sum) / float64(g.n)
		result = append(result, CaseAvgDuration{
			TestCaseID:        g.id,
			TestCaseDesc:      g.desc,
			FileName:          g.file,
			URL:               g.url,
			AverageDurationMs: avg,
			ExecutionCount:    g.n,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if order == "asc" {
			return result[i].AverageDurationMs < result[j].AverageDurationMs
		}
		return result[i].AverageDurationMs > result[j].AverageDurationMs
	})

	// 调试日志：raw_records 为读取到的全部记录数，result 为排序后返回的结果数
	logger.Debug("查询平均耗时",
		zap.Int("原始记录数", len(records)),
		zap.String("排序方式", order),
		zap.Int("结果条数", len(result)))
	return result, nil
}

// ConsecutiveFailureInfo 表示用例连续失败情况
type ConsecutiveFailureInfo struct {
	TestCaseID    string
	TestCaseDesc  string
	FileName      string
	URL           string
	RecentResults []bool
	FailCount     int
	LastExecuted  string
}

// GetConsecutiveFailures 获取每个用例最近N次执行结果，返回连续失败达到阈值的用例
func GetConsecutiveFailures(n int) ([]ConsecutiveFailureInfo, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return nil, err
	}

	type recInfo struct {
		success bool
		time    string
	}
	groups := make(map[string][]recInfo)
	meta := make(map[string]struct{ desc, file, url string })

	for _, rec := range records {
		if len(rec) < 7 {
			continue
		}
		id := rec[0]
		meta[id] = struct{ desc, file, url string }{rec[1], rec[2], rec[3]}
		groups[id] = append(groups[id], recInfo{
			success: parseSuccess(rec[5]),
			time:    rec[6],
		})
	}

	var alerts []ConsecutiveFailureInfo
	for id, hist := range groups {
		sort.Slice(hist, func(i, j int) bool {
			return hist[i].time > hist[j].time
		})
		window := hist
		if len(window) > n {
			window = window[:n]
		}
		if len(window) < n {
			// 调试日志：该用例最近执行历史不足 n 轮，无法判定为连续失败，跳过
			logger.Debug("用例历史不足阈值，跳过",
				zap.String("用例ID", id),
				zap.Int("历史记录数", len(hist)),
				zap.Int("窗口长度", len(window)),
				zap.Int("阈值", n))
			continue
		}
		allFail := true
		for _, r := range window {
			if r.success {
				allFail = false
				break
			}
		}
		if !allFail {
			continue
		}
		m := meta[id]
		info := ConsecutiveFailureInfo{
			TestCaseID:   id,
			TestCaseDesc: m.desc,
			FileName:     m.file,
			URL:          m.url,
			FailCount:    len(window),
			LastExecuted: window[0].time,
		}
		for i := len(window) - 1; i >= 0; i-- {
			info.RecentResults = append(info.RecentResults, window[i].success)
		}
		alerts = append(alerts, info)
	}

	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].FailCount > alerts[j].FailCount
	})

	// 调试日志：raw_records 为全部用例记录数，threshold 为连续失败判定阈值，result 为最终告警数
	// 若 result 远小于预期，通常是历史记录不足 threshold 轮，可查看下方 per-case 调试日志
	logger.Debug("查询连续失败告警",
		zap.Int("原始记录数", len(records)),
		zap.Int("阈值", n),
		zap.Int("结果条数", len(alerts)))
	return alerts, nil
}

// GetExecutionHistory 获取指定用例最近 limit 次执行历史
func GetExecutionHistory(testCaseID string, limit int) ([]map[string]interface{}, error) {
	mu.RLock()
	defer mu.RUnlock()

	if dataDir == "" {
		return nil, fmt.Errorf("storage not initialized")
	}

	_, records, err := readRecords(executionCSVPath())
	if err != nil {
		return nil, err
	}

	var history []map[string]interface{}
	for _, rec := range records {
		if len(rec) < 7 {
			continue
		}
		if rec[0] != testCaseID {
			continue
		}
		history = append(history, map[string]interface{}{
			"test_case_id":   rec[0],
			"test_case_desc": rec[1],
			"file_name":      rec[2],
			"url":            rec[3],
			"duration_ms":    parseInt64(rec[4]),
			"success":        parseSuccess(rec[5]),
			"executed_at":    rec[6],
		})
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i]["executed_at"].(string) > history[j]["executed_at"].(string)
	})

	if len(history) > limit {
		history = history[:limit]
	}
	return history, nil
}