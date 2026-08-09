// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

// MonthReport 表示月报内容
type MonthReport struct {
	StartDate  string
	EndDate    string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
	Template   ReportTemplate
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

	if len(stats) == 0 {
		for d := startDate; d <= endDate; d = nextDay(d) {
			liveSummary, err := storage.GetDailySummaryFromExecutions(d)
			if err == nil && liveSummary != nil {
				stats = append(stats, *liveSummary)
			}
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

	return &MonthReport{
		StartDate:  startDate,
		EndDate:    endDate,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
		Template:   DefaultMonthTemplate(),
	}, nil
}

// GenerateMonthReportWithTemplate 使用指定模板生成本月报告
func GenerateMonthReportWithTemplate(deviceName string, consecutiveFailN int, topSlowN int, tmpl ReportTemplate) (*MonthReport, error) {
	report, err := GenerateMonthReport(deviceName, consecutiveFailN, topSlowN)
	if err != nil {
		return nil, err
	}
	report.Template = tmpl
	return report, nil
}

// FormatMonthReport 格式化月报为纯文本
func (r *MonthReport) FormatMonthReport() string {
	return formatReportText(r.Template, r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}

// FormatMonthReportHTML 格式化月报为HTML（用于邮件）
func (r *MonthReport) FormatMonthReportHTML() string {
	return formatReportHTML(r.Template, r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, r.SlowCases, r.AlertCases)
}
