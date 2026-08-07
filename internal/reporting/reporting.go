// Package reporting 提供周报月报年报生成、趋势图（ASCII）和慢接口分析功能
package reporting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

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