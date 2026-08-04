// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

const (
	barWidth = 40
)

// WeekReport 表示周报内容
type WeekReport struct {
	StartDate  string
	EndDate    string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
}

// DayReport 表示日报内容
type DayReport struct {
	Date       string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
}

// MonthReport 表示月报内容
type MonthReport struct {
	StartDate  string
	EndDate    string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
}

// YearReport 表示年报内容
type YearReport struct {
	StartDate  string
	EndDate    string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
}

// GenerateWeekReport 生成本周报告
func GenerateWeekReport(deviceName string, consecutiveFailN int, topSlowN int) (*WeekReport, error) {
	now := timeutil.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := now.AddDate(0, 0, -(weekday - 1))
	startDate := start.Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(startDate, endDate)
	if err != nil {
		return nil, err
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		alertCases = nil
	}

	return &WeekReport{
		StartDate:  startDate,
		EndDate:    endDate,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
	}, nil
}

// GenerateDayReport 生成本日报告
func GenerateDayReport(deviceName string, consecutiveFailN int, topSlowN int) (*DayReport, error) {
	now := timeutil.Now()
	dateStr := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(dateStr, dateStr)
	if err != nil {
		return nil, err
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		alertCases = nil
	}

	return &DayReport{
		Date:       dateStr,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
	}, nil
}

// GenerateMonthReport 生成本月报告
func GenerateMonthReport(deviceName string, consecutiveFailN int, topSlowN int) (*MonthReport, error) {
	now := timeutil.Now()
	startDate := now.Format("2006-01") + "-01"
	endDate := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(startDate, endDate)
	if err != nil {
		return nil, err
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		alertCases = nil
	}

	return &MonthReport{
		StartDate:  startDate,
		EndDate:    endDate,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
	}, nil
}

// GenerateYearReport 生成本年报告
func GenerateYearReport(deviceName string, consecutiveFailN int, topSlowN int) (*YearReport, error) {
	now := timeutil.Now()
	startDate := now.Format("2006") + "-01-01"
	endDate := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(startDate, endDate)
	if err != nil {
		return nil, err
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		alertCases = nil
	}

	return &YearReport{
		StartDate:  startDate,
		EndDate:    endDate,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
	}, nil
}

// FormatWeekReport 格式化周报为纯文本
func (r *WeekReport) FormatWeekReport() string {
	return formatReportText("周报", "本周", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatDayReport 格式化日报为纯文本
func (r *DayReport) FormatDayReport() string {
	return formatReportText("日报", "本日", r.Date, r.Date, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatMonthReport 格式化月报为纯文本
func (r *MonthReport) FormatMonthReport() string {
	return formatReportText("月报", "本月", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatYearReport 格式化年报为纯文本
func (r *YearReport) FormatYearReport() string {
	return formatReportText("年报", "本年", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

func formatReportText(reportType, periodLabel, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	var sb strings.Builder

	divider := strings.Repeat("=", 68)
	thinDivider := strings.Repeat("-", 68)

	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  pipetGo 测试%s  (%s ~ %s)\n", reportType, startDate, endDate))
	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  设备: %s\n", deviceName))
	sb.WriteString(fmt.Sprintf("  生成时间: %s\n\n", timeutil.FormatDateTime(timeutil.Now())))

	// 汇总统计
	if len(dailyStats) > 0 {
		total, passed, failed, skipped := 0, 0, 0, 0
		var totalDur time.Duration
		for _, d := range dailyStats {
			total += d.Total
			passed += d.Passed
			failed += d.Failed
			skipped += d.Skipped
			totalDur += time.Duration(d.TotalDurationMs) * time.Millisecond
		}
		sb.WriteString(fmt.Sprintf("  %s汇总:\n", periodLabel))
		sb.WriteString(fmt.Sprintf("    总执行: %d | 通过: %d | 失败: %d | 跳过: %d\n", total, passed, failed, skipped))
		passRate := float64(0)
		if passed+failed > 0 {
			passRate = float64(passed) / float64(passed+failed) * 100
		}
		sb.WriteString(fmt.Sprintf("    通过率: %.2f%% | 总耗时: %v\n", passRate, totalDur))
	}

	// 用例增长趋势图
	sb.WriteString("\n" + thinDivider + "\n")
	sb.WriteString("  [用例增长趋势图]\n")
	sb.WriteString(renderCaseGrowthChart(dailyStats))

	// 错误率趋势图
	sb.WriteString("\n" + thinDivider + "\n")
	sb.WriteString("  [错误率趋势图]\n")
	sb.WriteString(renderErrorRateChart(dailyStats))

	// 慢接口排名
	if len(slowCases) > 0 {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  [最慢接口 TOP " + fmt.Sprintf("%d", len(slowCases)) + "]\n")
		sb.WriteString(renderSlowCases(slowCases))
	}

	// 连续失败告警
	if len(alertCases) > 0 {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  ⚠️  [连续失败告警]\n")
		sb.WriteString(renderAlertCases(alertCases))
	}

	sb.WriteString("\n" + divider + "\n")
	sb.WriteString("  来自 pipetGo 测试程序\n")
	sb.WriteString(divider + "\n")

	return sb.String()
}

// renderCaseGrowthChart 渲染用例增长趋势 ASCII 横向图
func renderCaseGrowthChart(stats []storage.DailySummary) string {
	if len(stats) == 0 {
		return "  (无数据)\n"
	}

	var sb strings.Builder
	barWidth := 40
	maxVal := 0
	for _, s := range stats {
		if s.UniqueCases > maxVal {
			maxVal = s.UniqueCases
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	for _, s := range stats {
		label := s.Date[5:] // MM-DD
		ratio := float64(s.UniqueCases) / float64(maxVal)
		barLen := int(ratio * float64(barWidth))
		bar := strings.Repeat("█", barLen)
		space := strings.Repeat("░", barWidth-barLen)
		sb.WriteString(fmt.Sprintf("  %s  %s%s %d\n", label, bar, space, s.UniqueCases))
	}
	return sb.String()
}

// renderErrorRateChart 渲染错误率趋势 ASCII 横向图
func renderErrorRateChart(stats []storage.DailySummary) string {
	if len(stats) == 0 {
		return "  (无数据)\n"
	}

	var sb strings.Builder
	barWidth := 40
	maxVal := 0.0
	for _, s := range stats {
		if s.ErrorRate > maxVal {
			maxVal = s.ErrorRate
		}
	}
	if maxVal < 5 {
		maxVal = 5
	}
	maxVal = math.Ceil(maxVal/5) * 5

	for _, s := range stats {
		label := s.Date[5:] // MM-DD
		ratio := s.ErrorRate / maxVal
		barLen := int(ratio * float64(barWidth))
		bar := strings.Repeat("█", barLen)
		space := strings.Repeat("░", barWidth-barLen)
		sb.WriteString(fmt.Sprintf("  %s  %s%s %.1f%%\n", label, bar, space, s.ErrorRate))
	}
	return sb.String()
}

// renderSlowCases 渲染慢接口排名（ASCII 进度条）
func renderSlowCases(cases []storage.CaseAvgDuration) string {
	if len(cases) == 0 {
		return "  (无数据)\n"
	}

	var sb strings.Builder
	maxDur := cases[0].AverageDurationMs
	if maxDur <= 0 {
		maxDur = 1
	}

	sb.WriteString(fmt.Sprintf("  %-5s %-40s %10s %8s %s\n", "排名", "接口", "平均耗时", "执行次数", "耗时占比"))
	sb.WriteString("  " + strings.Repeat("-", 66) + "\n")

	var totalMs float64
	for _, c := range cases {
		totalMs += c.AverageDurationMs * float64(c.ExecutionCount)
	}

	for i, c := range cases {
		ratio := c.AverageDurationMs / maxDur
		barLen := int(ratio * float64(barWidth))
		bar := strings.Repeat("█", barLen)
		space := strings.Repeat("░", barWidth-barLen)

		var timeStr string
		ms := c.AverageDurationMs
		if ms >= 1000 {
			timeStr = fmt.Sprintf("%.2fs", ms/1000)
		} else {
			timeStr = fmt.Sprintf("%.0fms", ms)
		}

		desc := c.TestCaseDesc
		if desc == "" {
			desc = c.TestCaseID
		}
		if len(desc) > 38 {
			desc = desc[:35] + "..."
		}

		share := 0.0
		if totalMs > 0 {
			share = c.AverageDurationMs * float64(c.ExecutionCount) / totalMs * 100
		}

		sb.WriteString(fmt.Sprintf("  #%-4d %-40s %8s %6d  %s%s %5.1f%%\n",
			i+1, desc, timeStr, c.ExecutionCount, bar, space, share))
	}

	return sb.String()
}

// renderAlertCases 渲染连续失败告警
func renderAlertCases(alerts []storage.ConsecutiveFailureInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  以下用例连续 %d 轮失败，请重点关注:\n\n", len(alerts[0].RecentResults)))
	sb.WriteString(fmt.Sprintf("  %-20s %-35s %s\n", "用例ID", "描述", "最近执行时间"))
	sb.WriteString("  " + strings.Repeat("-", 68) + "\n")

	for _, a := range alerts {
		desc := a.TestCaseDesc
		if len(desc) > 33 {
			desc = desc[:30] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-20s %-35s %s\n", a.TestCaseID, desc, a.LastExecuted))
	}

	return sb.String()
}

// FormatWeekReportHTML 格式化周报为HTML（用于邮件）
func (r *WeekReport) FormatWeekReportHTML() string {
	return formatReportHTML("周报", "本周", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatDayReportHTML 格式化日报为HTML（用于邮件）
func (r *DayReport) FormatDayReportHTML() string {
	return formatReportHTML("日报", "本日", r.Date, r.Date, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatMonthReportHTML 格式化月报为HTML（用于邮件）
func (r *MonthReport) FormatMonthReportHTML() string {
	return formatReportHTML("月报", "本月", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatYearReportHTML 格式化年报为HTML（用于邮件）
func (r *YearReport) FormatYearReportHTML() string {
	return formatReportHTML("年报", "本年", r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

func formatReportHTML(reportType, periodLabel, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	var sb strings.Builder

	sb.WriteString(`<html><body style="font-family: monospace; font-size: 13px; background: #1a1a2e; color: #e0e0e0; padding: 20px;">`)
	sb.WriteString(fmt.Sprintf(`<h2 style="color: #00d4ff;">pipetGo 测试%s (%s ~ %s)</h2>`, reportType, startDate, endDate))
	sb.WriteString(fmt.Sprintf(`<p>设备: <b>%s</b> | 生成时间: %s</p>`, deviceName, timeutil.FormatDateTime(timeutil.Now())))

	if len(dailyStats) > 0 {
		total, passed, failed, skipped := 0, 0, 0, 0
		var totalDur time.Duration
		for _, d := range dailyStats {
			total += d.Total
			passed += d.Passed
			failed += d.Failed
			skipped += d.Skipped
			totalDur += time.Duration(d.TotalDurationMs) * time.Millisecond
		}
		passRate := float64(0)
		if passed+failed > 0 {
			passRate = float64(passed) / float64(passed+failed) * 100
		}
		sb.WriteString(fmt.Sprintf(`<h3>%s汇总</h3>`, periodLabel))
		sb.WriteString(fmt.Sprintf(`<p>总执行: %d | 通过: %d | 失败: <span style="color: #ff4444;">%d</span> | 跳过: %d</p>`, total, passed, failed, skipped))
		sb.WriteString(fmt.Sprintf(`<p>通过率: %.2f%% | 总耗时: %v</p>`, passRate, totalDur))
	}

	sb.WriteString(`<h3>用例增长趋势</h3>`)
	sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
	sb.WriteString(renderCaseGrowthChart(dailyStats))
	sb.WriteString(`</pre>`)

	sb.WriteString(`<h3>错误率趋势</h3>`)
	sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
	sb.WriteString(renderErrorRateChart(dailyStats))
	sb.WriteString(`</pre>`)

	if len(slowCases) > 0 {
		sb.WriteString(`<h3>最慢接口 TOP ` + fmt.Sprintf("%d", len(slowCases)) + `</h3>`)
		sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
		sb.WriteString(renderSlowCases(slowCases))
		sb.WriteString(`</pre>`)
	}

	if len(alertCases) > 0 {
		sb.WriteString(`<h3 style="color: #ff4444;">⚠️ 连续失败告警</h3>`)
		sb.WriteString(`<pre style="background: #2a0000; padding: 10px; border-radius: 4px; color: #ff6666;">`)
		sb.WriteString(renderAlertCases(alertCases))
		sb.WriteString(`</pre>`)
	}

	sb.WriteString(`<p style="color: #666;">来自 pipetGo 测试程序</p>`)
	sb.WriteString(`</body></html>`)

	return sb.String()
}

// GenerateASCIIReport 生成综合ASCII报告（含进度条、百分比、耗时）
func GenerateASCIIReport(deviceName string, consecutiveFailN int, topSlowN int) string {
	var sb strings.Builder

	now := timeutil.Now()
	divider := strings.Repeat("=", 68)
	thinDivider := strings.Repeat("-", 68)

	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  pipetGo 综合测试报告 - %s\n", timeutil.FormatDateTime(now)))
	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  设备: %s\n\n", deviceName))

	// 慢接口分析
	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil || len(slowCases) == 0 {
		sb.WriteString("  [慢接口分析] (暂无足够数据)\n")
	} else {
		if topSlowN <= 0 {
			topSlowN = 10
		}
		if len(slowCases) > topSlowN {
			slowCases = slowCases[:topSlowN]
		}
		sb.WriteString("  [慢接口分析 - 耗时排名]\n")
		sb.WriteString(renderSlowCases(slowCases))
	}

	// 错误率趋势
	sb.WriteString("\n" + thinDivider + "\n")
	sb.WriteString("  [错误率趋势 (近30天)]\n")
	fromDate := now.AddDate(0, 0, -30).Format("2006-01-02")
	toDate := now.Format("2006-01-02")
	stats, _ := storage.GetDailySummaries(fromDate, toDate)
	sb.WriteString(renderErrorRateChart(stats))

	// 用例增长趋势
	sb.WriteString("\n" + thinDivider + "\n")
	sb.WriteString("  [用例增长趋势 (近30天)]\n")
	sb.WriteString(renderCaseGrowthChart(stats))

	// 连续失败告警
	if consecutiveFailN > 0 {
		alerts, err := storage.GetConsecutiveFailures(consecutiveFailN)
		if err == nil && len(alerts) > 0 {
			sb.WriteString("\n" + thinDivider + "\n")
			sb.WriteString("  ⚠️  [连续失败告警 (连续" + fmt.Sprintf("%d", consecutiveFailN) + "轮)]\n")
			sb.WriteString(renderAlertCases(alerts))
		}
	}

	// 总体统计
	if len(stats) > 0 {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  [近30天统计]\n")
		total, passed, failed := 0, 0, 0
		for _, d := range stats {
			total += d.Total
			passed += d.Passed
			failed += d.Failed
		}
		passRate := 0.0
		if passed+failed > 0 {
			passRate = float64(passed) / float64(passed+failed) * 100
		}
		sb.WriteString(fmt.Sprintf("  总执行: %d | 通过: %d | 失败: %d | 通过率: %.2f%%\n", total, passed, failed, passRate))

		// ASCII 进度条展示通过率
		barLen := int(passRate / 100 * 40)
		bar := strings.Repeat("█", barLen)
		space := strings.Repeat("░", 40-barLen)
		sb.WriteString(fmt.Sprintf("  [%s%s] %.1f%%\n", bar, space, passRate))
	}

	sb.WriteString("\n" + divider + "\n")
	sb.WriteString("  来自 pipetGo 测试程序\n")
	sb.WriteString(divider + "\n")

	return sb.String()
}

// GetCaseDurationList 获取所有用例耗时列表（按耗时降序）
func GetCaseDurationList(topN int) []storage.CaseAvgDuration {
	cases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		return nil
	}
	if topN > 0 && len(cases) > topN {
		return cases[:topN]
	}
	return cases
}

// FormatCaseDurationTable 格式化用例耗时表格
func FormatCaseDurationTable(cases []storage.CaseAvgDuration) string {
	if len(cases) == 0 {
		return "(无数据)"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%-5s %-20s %-30s %10s %8s %8s\n",
		"排名", "用例ID", "描述", "平均耗时", "执行次数", "耗时占比"))
	sb.WriteString(strings.Repeat("-", 85) + "\n")

	var totalMs float64
	for _, c := range cases {
		totalMs += c.AverageDurationMs * float64(c.ExecutionCount)
	}

	for i, c := range cases {
		var timeStr string
		ms := c.AverageDurationMs
		if ms >= 1000 {
			timeStr = fmt.Sprintf("%.2fs", ms/1000)
		} else {
			timeStr = fmt.Sprintf("%.0fms", ms)
		}
		desc := c.TestCaseDesc
		if len(desc) > 28 {
			desc = desc[:25] + "..."
		}
		share := 0.0
		if totalMs > 0 {
			share = c.AverageDurationMs * float64(c.ExecutionCount) / totalMs * 100
		}
		sb.WriteString(fmt.Sprintf("%-5d %-20s %-30s %10s %8d %7.1f%%\n",
			i+1, c.TestCaseID, desc, timeStr, c.ExecutionCount, share))
	}
	return sb.String()
}

// SortByDuration sorts cases by average duration
func SortByDuration(cases []storage.CaseAvgDuration, asc bool) {
	sort.Slice(cases, func(i, j int) bool {
		if asc {
			return cases[i].AverageDurationMs < cases[j].AverageDurationMs
		}
		return cases[i].AverageDurationMs > cases[j].AverageDurationMs
	})
}
