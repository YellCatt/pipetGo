package reporting

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"pipetGo/internal/storage"
	"pipetGo/internal/timeutil"
)

//go:embed templates/text/*.tmpl
var textTmplFS embed.FS

var textTmpl *template.Template

func init() {
	funcMap := template.FuncMap{
		"formatBytes": formatBytes,
	}
	textTmpl = template.Must(template.New("report").Funcs(funcMap).ParseFS(textTmplFS, "templates/text/*.tmpl"))
}

// formatBytes 格式化字节数为可读字符串
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ============================================================================
// 启动横幅数据结构
// ============================================================================

// StartupHeaderData 启动头部模板数据
type StartupHeaderData struct {
	StartTime     string
	DeviceName    string
	LogFile       string
	StartTimeRaw  string
	DeviceNameRaw string
	LogFileRaw    string
}

// StartupStatsData 测试统计模板数据
type StartupStatsData struct {
	TotalCountLine               string
	ChainCountLine               string
	IndependentCountLine         string
	HasTags                      bool
	TagsLine                     string
	ExecutedCountLine            string
	ExecutedChainCountLine       string
	ExecutedIndependentCountLine string
	NoTagExecutedLine            string
	EstimatedDurationLine        string
	HasMultipleRounds            bool
	RoundsLine                   string

	// HTML 模板用原始值
	TotalCount               int
	ChainCount               int
	IndependentCount         int
	TagsStr                  string
	ExecutedCount            int
	ExecutedChainCount       int
	ExecutedIndependentCount int
	EstimatedDuration        string
	Rounds                   int
	IntervalMs               int

	StartTimeRaw  string
	DeviceNameRaw string
	LogFileRaw    string
}

// BuildStartupHeaderData 构建启动头部数据
func BuildStartupHeaderData(deviceName, logFile string) StartupHeaderData {
	now := timeutil.FormatDateTime(timeutil.Now())
	return StartupHeaderData{
		StartTime:     padRight(now, 43),
		DeviceName:    padRight(deviceName, 43),
		LogFile:       padRight(logFile, 43),
		StartTimeRaw:  now,
		DeviceNameRaw: deviceName,
		LogFileRaw:    logFile,
	}
}

// BuildStartupStatsData 构建测试统计数据
func BuildStartupStatsData(totalCount, chainCount, independentCount int, tags []string, executedCount, executedChainCount, executedIndependentCount int, estimatedDuration string, rounds int, intervalMs int) StartupStatsData {
	hasTags := len(tags) > 0
	tagsStr := strings.Join(tags, ", ")

	data := StartupStatsData{
		HasTags:                      hasTags,
		HasMultipleRounds:            rounds > 1,
		TotalCountLine:               padRight(fmt.Sprintf("%d", totalCount), 35),
		ChainCountLine:               padRight(fmt.Sprintf("%d", chainCount), 43),
		IndependentCountLine:         padRight(fmt.Sprintf("%d", independentCount), 43),
		ExecutedChainCountLine:       padRight(fmt.Sprintf("%d", executedChainCount), 43),
		ExecutedIndependentCountLine: padRight(fmt.Sprintf("%d", executedIndependentCount), 43),
		EstimatedDurationLine:        padRight(estimatedDuration, 41),

		// HTML 原始值
		TotalCount:               totalCount,
		ChainCount:               chainCount,
		IndependentCount:         independentCount,
		TagsStr:                  tagsStr,
		ExecutedCount:            executedCount,
		ExecutedChainCount:       executedChainCount,
		ExecutedIndependentCount: executedIndependentCount,
		EstimatedDuration:        estimatedDuration,
		Rounds:                   rounds,
		IntervalMs:               intervalMs,
	}

	if hasTags {
		data.TagsLine = padRight(tagsStr, 40)
		data.ExecutedCountLine = padRight(fmt.Sprintf("%d", executedCount), 36)
	} else {
		data.NoTagExecutedLine = padRight(fmt.Sprintf("%d", executedCount), 27)
	}

	if rounds > 1 {
		data.RoundsLine = fmt.Sprintf("%d 轮，每轮间隔 %dms", rounds, intervalMs)
		// 补齐到43字符宽度（含中文）
		data.RoundsLine = padRight(data.RoundsLine, 43)
	}

	return data
}

// padRight 右填充到指定宽度（处理中文字符）
func padRight(s string, width int) string {
	runes := []rune(s)
	actualWidth := 0
	for _, r := range runes {
		if r > 127 {
			actualWidth += 2
		} else {
			actualWidth++
		}
	}
	padLen := width - actualWidth
	if padLen <= 0 {
		padLen = 0
	}
	return s + strings.Repeat(" ", padLen)
}

// ============================================================================
// 报告数据结构
// ============================================================================

// ReportData 报告模板渲染数据
type ReportData struct {
	ReportType  string
	PeriodLabel string
	StartDate   string
	EndDate     string
	DeviceName  string
	GeneratedAt string

	ShowSummary   bool
	Total         int
	Passed        int
	Failed        int
	Skipped       int
	PassRate      float64
	TotalDuration string
	PassBar       string
	PassSpace     string

	ShowGrowth  bool
	GrowthChart string

	ShowError  bool
	ErrorChart string

	ShowSlow  bool
	SlowCount int
	SlowChart string

	ShowAlert  bool
	AlertChart string
}

// BuildReportData 从模板配置和数据构建 ReportData
// dailyStats 用于汇总统计, trendStats 用于趋势图（为空时回退到 dailyStats）
func BuildReportData(tmpl ReportTemplate, startDate, endDate, deviceName string, dailyStats []storage.DailySummary, trendStats []storage.DailySummary, slowCases []storage.CaseAvgDuration, alertCases []storage.ConsecutiveFailureInfo) ReportData {
	data := ReportData{
		ReportType:  tmpl.ReportType,
		PeriodLabel: tmpl.PeriodLabel,
		StartDate:   startDate,
		EndDate:     endDate,
		DeviceName:  deviceName,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),

		ShowSummary: tmpl.ShowSummary,
		ShowGrowth:  tmpl.ShowGrowth,
		ShowError:   tmpl.ShowError,
		ShowSlow:    tmpl.ShowSlow,
		ShowAlert:   tmpl.ShowAlert,
	}

	// 汇总统计：仅使用 dailyStats（当日/当周/当月汇总）
	if len(dailyStats) > 0 {
		var totalDur time.Duration
		for _, d := range dailyStats {
			data.Total += d.Total
			data.Passed += d.Passed
			data.Failed += d.Failed
			data.Skipped += d.Skipped
			totalDur += time.Duration(d.TotalDurationMs) * time.Millisecond
		}
		if data.Passed+data.Failed > 0 {
			data.PassRate = float64(data.Passed) / float64(data.Passed+data.Failed) * 100
		}
		data.TotalDuration = totalDur.String()

		barLen := int(data.PassRate / 100 * float64(barWidth))
		data.PassBar = strings.Repeat("█", barLen)
		data.PassSpace = strings.Repeat("░", barWidth-barLen)
	}

	// 趋势图：优先使用 trendStats，为空时回退到 dailyStats
	chartStats := trendStats
	if len(chartStats) == 0 {
		chartStats = dailyStats
	}

	// 用例增长趋势图
	if data.ShowGrowth {
		data.GrowthChart = renderCaseGrowthChart(chartStats)
	}

	// 错误率趋势图
	if data.ShowError {
		data.ErrorChart = renderErrorRateChart(chartStats)
	}

	// 慢接口排名
	if data.ShowSlow && len(slowCases) > 0 {
		data.SlowCount = len(slowCases)
		data.SlowChart = renderSlowCases(slowCases)
	}

	// 连续失败告警
	if data.ShowAlert && len(alertCases) > 0 {
		data.AlertChart = renderAlertCases(alertCases)
	}

	return data
}

// ============================================================================
// 模板渲染函数
// ============================================================================

// RenderTextReport 使用文本模板渲染报告
func RenderTextReport(tmplName string, data any) (string, error) {
	var buf bytes.Buffer
	if err := textTmpl.ExecuteTemplate(&buf, tmplName, data); err != nil {
		return "", fmt.Errorf("渲染文本模板 %s 失败: %w", tmplName, err)
	}
	return buf.String(), nil
}

// MustRenderText 渲染文本模板，失败时返回错误信息字符串
func MustRenderText(tmplName string, data any) string {
	result, err := RenderTextReport(tmplName, data)
	if err != nil {
		return fmt.Sprintf("模板渲染错误: %v", err)
	}
	return result
}