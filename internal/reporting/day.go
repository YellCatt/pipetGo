// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"pipetGo/internal/logger"
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"

	"go.uber.org/zap"
)

// DayReport 表示日报内容
type DayReport struct {
	Date       string
	DailyStats []storage.DailySummary // 当日汇总数据
	TrendStats []storage.DailySummary // 近N天趋势数据（用于趋势图）
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
	Template   ReportTemplate
}

// GenerateDayReport 生成本日报告
func GenerateDayReport(deviceName string, consecutiveFailN int, topSlowN int) (*DayReport, error) {
	now := timeutil.Now()
	dateStr := now.Format("2006-01-02")

	// 当日汇总数据
	stats, err := storage.GetDailySummaries(dateStr, dateStr)
	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		liveSummary, err := storage.GetDailySummaryFromExecutions(dateStr)
		if err != nil {
			logger.Warn("从执行记录实时计算当日汇总失败",
				zap.String("date", dateStr),
				zap.Error(err))
		} else if liveSummary != nil {
			stats = append(stats, *liveSummary)
		}
	}
	// 调试日志：确认当日汇总是否取到数据（为 0 时日报统计区为空）
	logger.Debug("日报数据获取 - 当日汇总", zap.Int("条数", len(stats)), zap.String("日期", dateStr))

	// 近N天趋势数据（用于趋势图）
	trendFrom := now.AddDate(0, 0, -(TrendDays - 1)).Format("2006-01-02")
	trendStats, err := storage.GetDailySummaries(trendFrom, dateStr)
	if err != nil {
		logger.Warn("获取趋势数据失败，趋势图将仅显示当日数据",
			zap.String("from", trendFrom),
			zap.String("to", dateStr),
			zap.Error(err))
		trendStats = stats
	}
	if len(trendStats) == 0 {
		trendStats = stats
	}
	// 调试日志：确认趋势数据量；为空时已在上方回退到当日数据
	logger.Debug("日报数据获取 - 趋势数据", zap.Int("条数", len(trendStats)), zap.String("起始", trendFrom), zap.String("结束", dateStr))

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取慢接口排行失败，日报将不包含慢接口数据", zap.Error(err))
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}
	// 调试日志：确认慢接口条数（已截断到 topSlowN）
	logger.Debug("日报数据获取 - 慢接口", zap.Int("条数", len(slowCases)), zap.Int("慢接口上限", topSlowN))

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		logger.Warn("获取连续失败告警失败，日报将不包含告警信息", zap.Error(err))
		alertCases = nil
	}
	// 调试日志：确认连续失败告警条数（consecutiveFailN 为判定阈值）
	logger.Debug("日报数据获取 - 连续失败告警", zap.Int("条数", len(alertCases)), zap.Int("连续失败阈值", consecutiveFailN))

	return &DayReport{
		Date:       dateStr,
		DailyStats: stats,
		TrendStats: trendStats,
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
	return formatReportText(r.Template, r.Date, r.Date, r.DeviceName, r.DailyStats, r.TrendStats, r.SlowCases, r.AlertCases)
}

// FormatDayReportHTML 格式化日报为HTML（用于邮件）
func (r *DayReport) FormatDayReportHTML() string {
	return formatReportHTML(r.Template, r.Date, r.Date, r.DeviceName, r.DailyStats, r.TrendStats, r.SlowCases, r.AlertCases)
}