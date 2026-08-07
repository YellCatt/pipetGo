// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"sort"
	"strings"

	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
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

	stats, _ := storage.GetDailySummaries(fromDate, toDate)

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil || len(slowCases) == 0 {
		slowCases = nil
	} else {
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
		if err == nil && len(alerts) > 0 {
			alertCases = alerts
		}
	}

	return formatReportText(tmpl, fromDate, toDate, deviceName, stats, slowCases, alertCases)
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
