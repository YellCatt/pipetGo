// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"sort"
	"strings"

	"pipetGo/internal/logger"
	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"

	"go.uber.org/zap"
)

// GenerateASCIIReport 生成综合ASCII报告（含进度条、百分比、耗时）
// 使用默认 ASCII 模板
func GenerateASCIIReport(deviceName string, consecutiveFailN int, topSlowN int) string {
	return GenerateASCIIReportWithTemplate(deviceName, consecutiveFailN, topSlowN, DefaultASCIITemplate())
}

// GenerateASCIIReportWithTemplate 使用指定模板生成综合ASCII报告
func GenerateASCIIReportWithTemplate(deviceName string, consecutiveFailN int, topSlowN int, tmpl ReportTemplate) string {
	now := timeutil.Now()
	fromDate := now.AddDate(0, 0, -30).Format("2006-01-02")
	toDate := now.Format("2006-01-02")

	stats, err := storage.GetDailySummaries(fromDate, toDate)
	if err != nil {
		logger.Warn("获取每日汇总失败，报告可能缺少历史数据",
			zap.String("from", fromDate),
			zap.String("to", toDate),
			zap.Error(err))
	}

	if len(stats) == 0 {
		for d := fromDate; d <= toDate; d = nextDay(d) {
			liveSummary, err := storage.GetDailySummaryFromExecutions(d)
			if err != nil {
				logger.Warn("从执行记录实时计算日汇总失败",
					zap.String("date", d),
					zap.Error(err))
			} else if liveSummary != nil {
				stats = append(stats, *liveSummary)
			}
		}
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取慢接口排行失败，报告将不包含慢接口数据", zap.Error(err))
		slowCases = nil
	} else if len(slowCases) == 0 {
		if topSlowN <= 0 {
			topSlowN = 10
		}
		if len(slowCases) > topSlowN {
			slowCases = slowCases[:topSlowN]
		}
	}

	var alertCases []storage.ConsecutiveFailureInfo
	if consecutiveFailN > 0 {
		alerts, err := storage.GetConsecutiveFailures(consecutiveFailN)
		if err != nil {
			logger.Warn("获取连续失败告警失败，报告将不包含告警信息",
				zap.Int("consecutive_fail_n", consecutiveFailN),
				zap.Error(err))
		} else if len(alerts) > 0 {
			alertCases = alerts
		}
	}

	return formatReportText(tmpl, fromDate, toDate, deviceName, stats, nil, slowCases, alertCases)
}

// GetCaseDurationList 获取所有用例耗时列表（按耗时降序）
func GetCaseDurationList(topN int) []storage.CaseAvgDuration {
	cases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取用例耗时列表失败", zap.Error(err))
		return nil
	}
	if topN > 0 && len(cases) > topN {
		return cases[:topN]
	}
	return cases
}

// FormatCaseDurationTable 格式化用例耗时表格
// 列宽根据实际内容动态计算，用例ID和描述保持完整不被截断。
func FormatCaseDurationTable(cases []storage.CaseAvgDuration) string {
	if len(cases) == 0 {
		return "(无数据)"
	}

	type rowData struct {
		id       string
		desc     string
		timeStr  string
		countStr string
		share    float64
	}

	rows := make([]rowData, len(cases))
	// totalMs = 所有接口耗时×执行次数的总和，用于计算每个接口耗时占比
	var totalMs float64
	for i, c := range cases {
		totalMs += c.AverageDurationMs * float64(c.ExecutionCount)

		// 耗时格式化：>=1s 显示为秒（带2位小数），否则显示为毫秒
		var timeStr string
		if c.AverageDurationMs >= 1000 {
			timeStr = fmt.Sprintf("%.2fs", c.AverageDurationMs/1000)
		} else {
			timeStr = fmt.Sprintf("%.0fms", c.AverageDurationMs)
		}

		// 描述为空时回退使用用例ID，保证该列有内容（此版本不截断，保持完整）
		desc := c.TestCaseDesc
		if desc == "" {
			desc = c.TestCaseID
		}

		rows[i] = rowData{
			id:       c.TestCaseID,
			desc:     desc,
			timeStr:  timeStr,
			countStr: fmt.Sprintf("%d", c.ExecutionCount),
		}
	}

	// 动态列宽：先以表头最小宽度初始化，再逐行取内容最大显示宽度
	// 使用 displayWidth 计算（中文字符占2列），保证等宽字体下真正对齐
	idW := max(displayWidth("用例ID"), 4)
	descW := max(displayWidth("描述"), 4)
	timeW := max(displayWidth("平均耗时"), 4)
	countW := max(displayWidth("执行次数"), 4)
	shareW := max(displayWidth("耗时占比"), 7)

	for i, r := range rows {
		// 耗时占比 = 该接口总耗时 / 全部接口总耗时 × 100%
		if totalMs > 0 {
			rows[i].share = cases[i].AverageDurationMs * float64(cases[i].ExecutionCount) / totalMs * 100
		}
		idW = max(idW, displayWidth(r.id))
		descW = max(descW, displayWidth(r.desc))
		timeW = max(timeW, displayWidth(r.timeStr))
		countW = max(countW, displayWidth(r.countStr))
	}

	// 排名列固定按 "#%-3d" 格式计算，占 4 个显示宽度；其余列按动态宽度
	sepWidth := 4 + 1 + idW + 1 + descW + 1 + timeW + 1 + countW + 1 + shareW

	// 调试日志：输出最终列宽，便于核对对齐异常
	logger.Debug("渲染平均耗时表格",
		zap.Int("行数", len(cases)),
		zap.Int("用例ID列宽", idW),
		zap.Int("描述列宽", descW),
		zap.Int("耗时列宽", timeW),
		zap.Int("次数列宽", countW),
		zap.Int("分隔线宽", sepWidth))

	var sb strings.Builder
	// 表头：各列用 padRight 填充到动态宽度，列间以单个空格分隔
	sb.WriteString(fmt.Sprintf("%-4s %s %s %s %s %s\n",
		"排名",
		padRight("用例ID", idW),
		padRight("描述", descW),
		padRight("平均耗时", timeW),
		padRight("执行次数", countW),
		padRight("耗时占比", shareW)))
	// 分隔线与表头等宽
	sb.WriteString(strings.Repeat("-", sepWidth) + "\n")

	// 数据行：与表头使用同一套列宽，保证逐列对齐；耗时占比右对齐 5.1f%%
	for i, r := range rows {
		sb.WriteString(fmt.Sprintf("#%-3d %s %s %s %s %5.1f%%\n",
			i+1,
			padRight(r.id, idW),
			padRight(r.desc, descW),
			padRight(r.timeStr, timeW),
			padRight(r.countStr, countW),
			r.share))
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