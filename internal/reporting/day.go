// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

// DayReport 表示日报内容
type DayReport struct {
	Date       string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
	Template   ReportTemplate
}

// GenerateDayReport 生成本日报告
func GenerateDayReport(deviceName string, consecutiveFailN int, topSlowN int) (*DayReport, error) {
	now := timeutil.Now()
	dateStr := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(dateStr, dateStr)
	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		liveSummary, err := storage.GetDailySummaryFromExecutions(dateStr)
		if err == nil && liveSummary != nil {
			stats = append(stats, *liveSummary)
		}
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
		Template:   DefaultDayTemplate(),
	}, nil
}

// GenerateDayReportWithTemplate 使用指定模板生成本日报告
func GenerateDayReportWithTemplate(deviceName string, consecutiveFailN int, topSlowN int, tmpl ReportTemplate) (*DayReport, error) {
	report, err := GenerateDayReport(deviceName, consecutiveFailN, topSlowN)
	if err != nil {
		return nil, err
	}
	report.Template = tmpl
	return report, nil
}

// FormatDayReport 格式化日报为纯文本
func (r *DayReport) FormatDayReport() string {
	return formatReportText(r.Template, r.Date, r.Date, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatDayReportHTML 格式化日报为HTML（用于邮件）
func (r *DayReport) FormatDayReportHTML() string {
	return formatReportHTML(r.Template, r.Date, r.Date, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}
