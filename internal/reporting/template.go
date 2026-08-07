// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"math"
	"strings"
	"time"

	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

const (
	barWidth = 40
)

// ReportTemplate 定义报告模板，控制报告中各模块的显示与顺序
type ReportTemplate struct {
	ReportType   string // 报告类型名称（日报/周报/月报/年报）
	PeriodLabel  string // 周期标签（本日/本周/本月/本年）
	ShowSummary  bool   // 是否显示汇总统计
	ShowGrowth   bool   // 是否显示用例增长趋势图
	ShowError    bool   // 是否显示错误率趋势图
	ShowSlow     bool   // 是否显示慢接口排名
	ShowAlert    bool   // 是否显示连续失败告警
}

// DefaultDayTemplate 返回日报默认模板
func DefaultDayTemplate() ReportTemplate {
	return ReportTemplate{
		ReportType:  "日报",
		PeriodLabel: "本日",
		ShowSummary: true,
		ShowGrowth:  true,
		ShowError:   true,
		ShowSlow:    true,
		ShowAlert:   true,
	}
}

// DefaultWeekTemplate 返回周报默认模板
func DefaultWeekTemplate() ReportTemplate {
	return ReportTemplate{
		ReportType:  "周报",
		PeriodLabel: "本周",
		ShowSummary: true,
		ShowGrowth:  true,
		ShowError:   true,
		ShowSlow:    true,
		ShowAlert:   true,
	}
}

// DefaultMonthTemplate 返回月报默认模板
func DefaultMonthTemplate() ReportTemplate {
	return ReportTemplate{
		ReportType:  "月报",
		PeriodLabel: "本月",
		ShowSummary: true,
		ShowGrowth:  true,
		ShowError:   true,
		ShowSlow:    true,
		ShowAlert:   true,
	}
}

// DefaultYearTemplate 返回年报默认模板
func DefaultYearTemplate() ReportTemplate {
	return ReportTemplate{
		ReportType:  "年报",
		PeriodLabel: "本年",
		ShowSummary: true,
		ShowGrowth:  true,
		ShowError:   true,
		ShowSlow:    true,
		ShowAlert:   true,
	}
}

// NewReportTemplate 根据配置参数创建报告模板
// reportType: 报告类型名称（日报/周报/月报/年报）
// periodLabel: 周期标签（本日/本周/本月/本年）
// showSummary, showGrowth, showError, showSlow, showAlert: 各模块显示开关
func NewReportTemplate(reportType, periodLabel string, showSummary, showGrowth, showError, showSlow, showAlert bool) ReportTemplate {
	return ReportTemplate{
		ReportType:  reportType,
		PeriodLabel: periodLabel,
		ShowSummary: showSummary,
		ShowGrowth:  showGrowth,
		ShowError:   showError,
		ShowSlow:    showSlow,
		ShowAlert:   showAlert,
	}
}

// formatReportText 使用模板格式化报告为纯文本
func formatReportText(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	var sb strings.Builder

	divider := strings.Repeat("=", 68)
	thinDivider := strings.Repeat("-", 68)

	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  pipetGo 测试%s  (%s ~ %s)\n", tmpl.ReportType, startDate, endDate))
	sb.WriteString(divider + "\n")
	sb.WriteString(fmt.Sprintf("  设备: %s\n", deviceName))
	sb.WriteString(fmt.Sprintf("  生成时间: %s\n\n", timeutil.FormatDateTime(timeutil.Now())))

	// 汇总统计
	if tmpl.ShowSummary && len(dailyStats) > 0 {
		total, passed, failed, skipped := 0, 0, 0, 0
		var totalDur time.Duration
		for _, d := range dailyStats {
			total += d.Total
			passed += d.Passed
			failed += d.Failed
			skipped += d.Skipped
			totalDur += time.Duration(d.TotalDurationMs) * time.Millisecond
		}
		sb.WriteString(fmt.Sprintf("  %s汇总:\n", tmpl.PeriodLabel))
		sb.WriteString(fmt.Sprintf("    总执行: %d | 通过: %d | 失败: %d | 跳过: %d\n", total, passed, failed, skipped))
		passRate := float64(0)
		if passed+failed > 0 {
			passRate = float64(passed) / float64(passed+failed) * 100
		}
		sb.WriteString(fmt.Sprintf("    通过率: %.2f%% | 总耗时: %v\n", passRate, totalDur))
	}

	// 用例增长趋势图
	if tmpl.ShowGrowth {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  [用例增长趋势图]\n")
		sb.WriteString(renderCaseGrowthChart(dailyStats))
	}

	// 错误率趋势图
	if tmpl.ShowError {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  [错误率趋势图]\n")
		sb.WriteString(renderErrorRateChart(dailyStats))
	}

	// 慢接口排名
	if tmpl.ShowSlow && len(slowCases) > 0 {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  [最慢接口 TOP " + fmt.Sprintf("%d", len(slowCases)) + "]\n")
		sb.WriteString(renderSlowCases(slowCases))
	}

	// 连续失败告警
	if tmpl.ShowAlert && len(alertCases) > 0 {
		sb.WriteString("\n" + thinDivider + "\n")
		sb.WriteString("  ⚠️  [连续失败告警]\n")
		sb.WriteString(renderAlertCases(alertCases))
	}

	sb.WriteString("\n" + divider + "\n")
	sb.WriteString("  来自 pipetGo 测试程序\n")
	sb.WriteString(divider + "\n")

	return sb.String()
}

// formatReportHTML 使用模板格式化报告为HTML
func formatReportHTML(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	var sb strings.Builder

	sb.WriteString(`<html><body style="font-family: monospace; font-size: 13px; background: #1a1a2e; color: #e0e0e0; padding: 20px;">`)
	sb.WriteString(fmt.Sprintf(`<h2 style="color: #00d4ff;">pipetGo 测试%s (%s ~ %s)</h2>`, tmpl.ReportType, startDate, endDate))
	sb.WriteString(fmt.Sprintf(`<p>设备: <b>%s</b> | 生成时间: %s</p>`, deviceName, timeutil.FormatDateTime(timeutil.Now())))

	if tmpl.ShowSummary && len(dailyStats) > 0 {
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
		sb.WriteString(fmt.Sprintf(`<h3>%s汇总</h3>`, tmpl.PeriodLabel))
		sb.WriteString(fmt.Sprintf(`<p>总执行: %d | 通过: %d | 失败: <span style="color: #ff4444;">%d</span> | 跳过: %d</p>`, total, passed, failed, skipped))
		sb.WriteString(fmt.Sprintf(`<p>通过率: %.2f%% | 总耗时: %v</p>`, passRate, totalDur))
	}

	if tmpl.ShowGrowth {
		sb.WriteString(`<h3>用例增长趋势</h3>`)
		sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
		sb.WriteString(renderCaseGrowthChart(dailyStats))
		sb.WriteString(`</pre>`)
	}

	if tmpl.ShowError {
		sb.WriteString(`<h3>错误率趋势</h3>`)
		sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
		sb.WriteString(renderErrorRateChart(dailyStats))
		sb.WriteString(`</pre>`)
	}

	if tmpl.ShowSlow && len(slowCases) > 0 {
		sb.WriteString(`<h3>最慢接口 TOP ` + fmt.Sprintf("%d", len(slowCases)) + `</h3>`)
		sb.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
		sb.WriteString(renderSlowCases(slowCases))
		sb.WriteString(`</pre>`)
	}

	if tmpl.ShowAlert && len(alertCases) > 0 {
		sb.WriteString(`<h3 style="color: #ff4444;">⚠️ 连续失败告警</h3>`)
		sb.WriteString(`<pre style="background: #2a0000; padding: 10px; border-radius: 4px; color: #ff6666;">`)
		sb.WriteString(renderAlertCases(alertCases))
		sb.WriteString(`</pre>`)
	}

	sb.WriteString(`<p style="color: #666;">来自 pipetGo 测试程序</p>`)
	sb.WriteString(`</body></html>`)

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