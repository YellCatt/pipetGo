// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"pipetGo/internal/logger"
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"

	"go.uber.org/zap"
)

// YearReport 表示年报内容
type YearReport struct {
	StartDate  string
	EndDate    string
	DailyStats []storage.DailySummary
	SlowCases  []storage.CaseAvgDuration
	AlertCases []storage.ConsecutiveFailureInfo
	DeviceName string
	Template   ReportTemplate
}

// GenerateYearReport 生成本年报告
func GenerateYearReport(deviceName string, consecutiveFailN int, topSlowN int) (*YearReport, error) {
	now := timeutil.Now()
	startDate := now.Format("2006") + "-01-01"
	endDate := now.Format("2006-01-02")

	logger.Debug("开始生成年报",
		zap.String("起始日期", startDate),
		zap.String("结束日期", endDate),
		zap.String("设备名", deviceName),
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN))

	stats, err := storage.GetDailySummaries(startDate, endDate)
	if err != nil {
		logger.Error("获取年度日汇总失败", zap.Error(err))
		return nil, err
	}

	logger.Debug("年报日汇总查询结果",
		zap.Int("汇总条数", len(stats)))

	if len(stats) == 0 {
		logger.Debug("年报无日汇总数据，尝试从执行记录实时计算",
			zap.String("起始日期", startDate),
			zap.String("结束日期", endDate))
		for d := startDate; d <= endDate; d = nextDay(d) {
			liveSummary, err := storage.GetDailySummaryFromExecutions(d)
			if err != nil {
				logger.Warn("从执行记录实时计算日汇总失败",
					zap.String("日期", d),
					zap.Error(err))
			} else if liveSummary != nil {
				logger.Debug("实时计算日汇总成功",
					zap.String("日期", d),
					zap.Int("总数", liveSummary.Total),
					zap.Int("通过", liveSummary.Passed),
					zap.Int("失败", liveSummary.Failed))
				stats = append(stats, *liveSummary)
			}
		}
		logger.Debug("年报实时计算完成", zap.Int("汇总条数", len(stats)))
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取慢接口排行失败，年报将不包含慢接口数据", zap.Error(err))
		slowCases = nil
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}
	logger.Debug("年报慢接口数据", zap.Int("条数", len(slowCases)))

	alertCases, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		logger.Warn("获取连续失败告警失败，年报将不包含告警信息", zap.Error(err))
		alertCases = nil
	}
	logger.Debug("年报告警数据", zap.Int("条数", len(alertCases)))

	logger.Info("年报数据汇总",
		zap.String("周期", startDate+" ~ "+endDate),
		zap.Int("日汇总条数", len(stats)),
		zap.Int("慢接口条数", len(slowCases)),
		zap.Int("告警条数", len(alertCases)))

	return &YearReport{
		StartDate:  startDate,
		EndDate:    endDate,
		DailyStats: stats,
		SlowCases:  slowCases,
		AlertCases: alertCases,
		DeviceName: deviceName,
		Template:   DefaultYearTemplate(),
	}, nil
}

// GenerateYearReportWithTemplate 使用指定模板生成本年报告
func GenerateYearReportWithTemplate(deviceName string, consecutiveFailN int, topSlowN int, tmpl ReportTemplate) (*YearReport, error) {
	report, err := GenerateYearReport(deviceName, consecutiveFailN, topSlowN)
	if err != nil {
		return nil, err
	}
	report.Template = tmpl
	return report, nil
}

// FormatYearReport 格式化年报为纯文本
func (r *YearReport) FormatYearReport() string {
	return formatReportText(r.Template, r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, nil, r.SlowCases, r.AlertCases)
}

// FormatYearReportHTML 格式化年报为HTML（用于邮件）
func (r *YearReport) FormatYearReportHTML() string {
	return formatReportHTML(r.Template, r.StartDate, r.EndDate, r.DeviceName, r.DailyStats, nil, r.SlowCases, r.AlertCases)
}