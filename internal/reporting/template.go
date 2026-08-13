// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"math"
	"strings"
	"time"

	"pipetGo/internal/storage"
)

const (
	barWidth  = 40
	TrendDays = 7 // 趋势图展示最近N天数据
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
// dailyStats 用于汇总统计, trendStats 用于趋势图（为空时使用 dailyStats）
func formatReportText(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, trendStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	data := BuildReportData(tmpl, startDate, endDate, deviceName, dailyStats, trendStats, slowCases, alertCases)
	return MustRenderText(tmplNameFromType(tmpl.ReportType), data)
}

// formatReportHTML 邮件用纯文本报告（移动端友好）
// dailyStats 用于汇总统计, trendStats 用于趋势图（为空时使用 dailyStats）
func formatReportHTML(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, trendStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) string {
	data := BuildReportData(tmpl, startDate, endDate, deviceName, dailyStats, trendStats, slowCases, alertCases)
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

// truncateDesc 截断描述到指定显示宽度（中文字符算2列），超出部分用"..."替代
func truncateDesc(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return s
	}
	runes := []rune(s)
	width := 0
	limit := maxWidth - 3 // 预留"..."的宽度
	for i, r := range runes {
		w := 1
		if r > 127 {
			w = 2
		}
		if width+w > limit {
			if i > 0 {
				return string(runes[:i]) + "..."
			}
			return string(runes[:i]) + "..."
		}
		width += w
	}
	return s
}

// renderSlowCases 渲染慢接口排名（ASCII 进度条）
func renderSlowCases(cases []storage.CaseAvgDuration) string {
	if len(cases) == 0 {
		return "  (无数据)\n"
	}

	maxDur := cases[0].AverageDurationMs
	if maxDur <= 0 {
		maxDur = 1
	}

	type slowRow struct {
		id       string
		desc     string
		timeStr  string
		countStr string
		barLen   int
		share    float64
	}

	rows := make([]slowRow, len(cases))
	var totalMs float64
	for _, c := range cases {
		totalMs += c.AverageDurationMs * float64(c.ExecutionCount)
	}

	for i, c := range cases {
		desc := c.TestCaseDesc
		if desc == "" {
			desc = c.TestCaseID
		}

		var timeStr string
		if c.AverageDurationMs >= 1000 {
			timeStr = fmt.Sprintf("%.2fs", c.AverageDurationMs/1000)
		} else {
			timeStr = fmt.Sprintf("%.0fms", c.AverageDurationMs)
		}

		share := 0.0
		if totalMs > 0 {
			share = c.AverageDurationMs * float64(c.ExecutionCount) / totalMs * 100
		}

		barLen := int(c.AverageDurationMs / maxDur * float64(barWidth))
		if barLen > barWidth {
			barLen = barWidth
		}

		rows[i] = slowRow{
			id:       c.TestCaseID,
			desc:     desc,
			timeStr:  timeStr,
			countStr: fmt.Sprintf("%d", c.ExecutionCount),
			barLen:   barLen,
			share:    share,
		}
	}

	// 根据实际内容动态计算列宽，保证完整显示并对齐
	rankW := max(displayWidth("排名"), 4)
	idW := max(displayWidth("用例ID"), 4)
	descW := max(displayWidth("描述"), 4)
	timeW := max(displayWidth("平均耗时"), 4)
	countW := max(displayWidth("执行次数"), 4)
	shareW := max(displayWidth("耗时占比"), 7)

	for _, r := range rows {
		idW = max(idW, displayWidth(r.id))
		descW = max(descW, displayWidth(r.desc))
		timeW = max(timeW, displayWidth(r.timeStr))
		countW = max(countW, displayWidth(r.countStr))
	}

	shareAreaW := barWidth + 1 + shareW
	sepWidth := rankW + 1 + idW + 1 + descW + 1 + timeW + 1 + countW + 1 + shareAreaW

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %s %s %s %s %s %s\n",
		padRight("排名", rankW),
		padRight("用例ID", idW),
		padRight("描述", descW),
		padRight("平均耗时", timeW),
		padRight("执行次数", countW),
		padRight("耗时占比", shareAreaW)))
	sb.WriteString("  " + strings.Repeat("-", sepWidth) + "\n")

	for i, r := range rows {
		bar := strings.Repeat("█", r.barLen)
		space := strings.Repeat("░", barWidth-r.barLen)
		shareStr := fmt.Sprintf("%6.1f%%", r.share)

		sb.WriteString(fmt.Sprintf("  %s %s %s %s %s %s %s\n",
			padRight(fmt.Sprintf("#%02d", i+1), rankW),
			padRight(r.id, idW),
			padRight(r.desc, descW),
			padLeft(r.timeStr, timeW),
			padLeft(r.countStr, countW),
			bar+space,
			padLeft(shareStr, shareW)))
	}

	return sb.String()
}

// renderAlertCases 渲染连续失败告警
func renderAlertCases(alerts []storage.ConsecutiveFailureInfo) string {
	if len(alerts) == 0 {
		return "  (无连续失败告警)\n"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  以下用例连续 %d 轮失败，请重点关注:\n\n", len(alerts[0].RecentResults)))

	idWidth := 18
	descWidth := 30
	timeWidth := 19 // "最近执行时间" 视觉宽度

	sb.WriteString(fmt.Sprintf("  %s %s %s\n",
		padRight("用例ID", idWidth),
		padRight("描述", descWidth),
		"最近执行时间"))
	sb.WriteString("  " + strings.Repeat("-", idWidth+1+descWidth+1+timeWidth) + "\n")

	for _, a := range alerts {
		desc := a.TestCaseDesc
		if desc == "" {
			desc = a.TestCaseID
		}
		desc = truncateDesc(desc, descWidth)
		sb.WriteString(fmt.Sprintf("  %s %s %s\n",
			padRight(truncateDesc(a.TestCaseID, idWidth), idWidth),
			padRight(desc, descWidth),
			a.LastExecuted))
	}

	return sb.String()
}

// nextDay 返回指定日期的下一天（格式：2006-01-02）
func nextDay(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}