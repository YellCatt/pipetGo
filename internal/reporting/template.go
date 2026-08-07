// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"math"
	"strings"

	"pipetGo/internal/storage"
)

const (
	barWidth = 40
)

// ReportTemplate 定义报告模板，控制报告中各模块的显示与顺序
type ReportTemplate struct {
	ReportType  string // 报告类型名称（日报/周报/月报/年报）
	PeriodLabel string // 周期标签（本日/本周/本月/本年）
	ShowSummary bool   // 是否显示汇总统计
	ShowGrowth  bool   // 是否显示用例增长趋势图
	ShowError   bool   // 是否显示错误率趋势图
	ShowSlow    bool   // 是否显示慢接口排名
	ShowAlert   bool   // 是否显示连续失败告警
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

// DefaultASCIITemplate 返回ASCII综合报告默认模板
func DefaultASCIITemplate() ReportTemplate {
	return ReportTemplate{
		ReportType:  "综合报告",
		PeriodLabel: "近30天",
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
	data := BuildReportData(tmpl, startDate, endDate, deviceName, dailyStats, slowCases, alertCases)
	return MustRenderText(tmplNameFromType(tmpl.ReportType), data)
}

// formatReportHTML 邮件用纯文本报告（移动端友好）
func formatReportHTML(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	data := BuildReportData(tmpl, startDate, endDate, deviceName, dailyStats, slowCases, alertCases)
	text := MustRenderText(tmplNameFromType(tmpl.ReportType), data)
	return "<pre style=\"font-size: 12px; line-height: 1.4; white-space: pre-wrap; word-wrap: break-word;\">" + text + "</pre>"
}

// tmplNameFromType 根据报告类型返回模板名称
func tmplNameFromType(reportType string) string {
	switch reportType {
	case "日报":
		return "daily"
	case "周报":
		return "weekly"
	case "月报":
		return "monthly"
	case "年报":
		return "yearly"
	case "综合报告":
		return "ascii"
	default:
		return "daily"
	}
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
