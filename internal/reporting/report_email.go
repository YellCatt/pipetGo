package reporting

import (
	"fmt"
	"strings"
	"time"

	"pipetGo/internal/email"
	"pipetGo/internal/logger"
	"pipetGo/internal/storage"
	"pipetGo/internal/testcase"
	"pipetGo/internal/timeutil"

	"go.uber.org/zap"
)

func canSendEmail() bool {
	cfg := email.GetConfig()
	logger.Debug("检查邮件配置",
		zap.Bool("启用", cfg.Enabled),
		zap.String("发件人", cfg.FromEmail),
		zap.Int("收件人数", len(cfg.ToEmail)),
		zap.Bool("有授权码", cfg.AuthCode != ""),
		zap.String("SMTP服务器", cfg.SMTPServer),
		zap.Int("SMTP端口", cfg.SMTPPort))
	if !cfg.Enabled {
		logger.Warn("邮件发送功能已禁用")
		return false
	}
	if cfg.FromEmail == "" || len(cfg.ToEmail) == 0 || cfg.AuthCode == "" {
		logger.Warn("邮件配置不完整",
			zap.Bool("是否启用", cfg.Enabled),
			zap.String("发件人", cfg.FromEmail),
			zap.Int("收件人数", len(cfg.ToEmail)),
			zap.Bool("是否有授权码", cfg.AuthCode != ""))
		return false
	}
	logger.Debug("邮件配置检查通过")
	return true
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// renderFailureTable 渲染失败用例表格，列宽根据实际内容动态计算，保证中英文混排对齐。
// withMarker 为 true 时，连续失败用例在错误信息末尾追加 " [连续失败]" 标记。
// alertIDs 为连续失败用例 ID 集合（withMarker 为 true 时使用）。
func renderFailureTable(aggregated []testcase.TestResult, withMarker bool, alertIDs map[string]bool) string {
	const (
		idMaxW   = 30 // ID 列最大显示宽度
		descMaxW = 40 // 描述列最大显示宽度
		errMaxW  = 55 // 错误信息列最大显示宽度
	)

	type failRow struct {
		id     string
		desc   string
		status string
		durStr string
		err    string
		marker string
	}

	var rows []failRow
	for _, r := range aggregated {
		if !r.Passed && !r.TestCase.Skip {
			marker := ""
			if withMarker && alertIDs[r.TestCase.ID] {
				marker = " [连续失败]"
			}
			rows = append(rows, failRow{
				id:     r.TestCase.ID,
				desc:   r.TestCase.Desc,
				status: "FAIL",
				durStr: r.Duration.String(),
				err:    r.Error,
				marker: marker,
			})
		}
	}
	if len(rows) == 0 {
		return "(无失败用例)\n"
	}

	// 计算各列所需最大显示宽度（含表头），并加上本列上限以避免过宽
	idW := displayWidth("ID")
	descW := displayWidth("描述")
	statusW := displayWidth("状态")
	durW := displayWidth("耗时")
	errW := displayWidth("错误信息")

	for _, r := range rows {
		idW = max(idW, displayWidth(truncateDesc(r.id, idMaxW)))
		descW = max(descW, displayWidth(truncateDesc(r.desc, descMaxW)))
		statusW = max(statusW, displayWidth(r.status))
		durW = max(durW, displayWidth(r.durStr))
		errW = max(errW, displayWidth(truncateDesc(r.err, errMaxW)))
	}
	// 标记列可能额外占用宽度，错误信息列需预留
	markerMaxW := displayWidth(" [连续失败]")
	errW = max(errW+markerMaxW, displayWidth("错误信息")+markerMaxW)

	idW = min(idW, idMaxW)
	descW = min(descW, descMaxW)
	errW = min(errW, errMaxW+markerMaxW)

	sepWidth := idW + 1 + descW + 1 + statusW + 1 + durW + 1 + errW

	var sb strings.Builder
	sb.WriteString("-" + strings.Repeat("-", sepWidth) + "\n")
	sb.WriteString(fmt.Sprintf("%s %s %s %s %s\n",
		padRight("ID", idW),
		padRight("描述", descW),
		padRight("状态", statusW),
		padRight("耗时", durW),
		padRight("错误信息", errW)))
	sb.WriteString("-" + strings.Repeat("-", sepWidth) + "\n")

	for _, r := range rows {
		errCell := truncateDesc(r.err, errMaxW) + r.marker
		sb.WriteString(fmt.Sprintf("%s %s %s %s %s\n",
			padRight(truncateDesc(r.id, idMaxW), idW),
			padRight(truncateDesc(r.desc, descMaxW), descW),
			padRight(r.status, statusW),
			padRight(r.durStr, durW),
			padRight(errCell, errW)))
	}
	sb.WriteString("-" + strings.Repeat("-", sepWidth) + "\n")
	return sb.String()
}

func GenerateTestReportContent(results []testcase.TestResult) string {
	var sb strings.Builder

	now := timeutil.Now()

	logger.Debug("生成测试报告内容",
		zap.Int("结果数", len(results)))

	sb.WriteString(fmt.Sprintf("===== 测试报告 =====\n\n"))
	sb.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(now)))
	sb.WriteString(fmt.Sprintf("测试设备: %s\n", email.GetDeviceName()))

	chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped, totalDuration := testcase.SummarizeResultsByType(results)

	totalPassed := chainPassed + independentPassed
	totalFailed := chainFailed + independentFailed
	totalSkipped := chainSkipped + independentSkipped

	aggregated := testcase.AggregateResultsByFile(results)

	sb.WriteString(fmt.Sprintf("测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  总测试数: %d\n", totalPassed+totalFailed+totalSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", totalPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", totalFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", totalSkipped))
	sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(totalPassed)/float64(totalPassed+totalFailed)*100))
	sb.WriteString(fmt.Sprintf("  总耗时:   %v\n\n", totalDuration))

	sb.WriteString(fmt.Sprintf("单例测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  测试数:   %d\n", independentPassed+independentFailed+independentSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", independentPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", independentFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", independentSkipped))
	if independentPassed+independentFailed > 0 {
		sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(independentPassed)/float64(independentPassed+independentFailed)*100))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("链式测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  测试数:   %d\n", chainPassed+chainFailed+chainSkipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", chainPassed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", chainFailed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", chainSkipped))
	if chainPassed+chainFailed > 0 {
		sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(chainPassed)/float64(chainPassed+chainFailed)*100))
	}
	sb.WriteString("\n")

	if len(aggregated) > 0 {
		sb.WriteString("测试详情:\n")
		sb.WriteString(renderFailureTable(aggregated, false, nil))
	}

	sb.WriteString("\n===== 报告结束 =====\n")
	sb.WriteString("来自 pipetGo 测试程序")

	return sb.String()
}

func SendTestReportEmail(results []testcase.TestResult) error {
	if !canSendEmail() {
		return nil
	}

	subject := fmt.Sprintf("【测试报告】pipetGo - %s - %s", email.GetDeviceName(), timeutil.FormatDateTime(timeutil.Now()))
	body := GenerateTestReportContent(results)

	logger.Info("开始发送测试报告邮件",
		zap.String("主题", subject),
		zap.Int("结果数", len(results)))
	return email.SendEmail(subject, body)
}

func SendErrorReportEmail(errorMessage string) error {
	if !canSendEmail() {
		return nil
	}

	subject := fmt.Sprintf("【测试异常】pipetGo - %s - %s", email.GetDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString("===== 测试异常报告 =====\n\n")
	body.WriteString(fmt.Sprintf("发生时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", email.GetDeviceName()))
	body.WriteString(fmt.Sprintf("\n异常信息:\n"))
	body.WriteString(fmt.Sprintf("  %s\n", errorMessage))
	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	logger.Info("开始发送异常报告邮件",
		zap.String("主题", subject))
	return email.SendEmail(subject, body.String())
}

func SendTestStartEmail(testCaseCount, chainCount, independentCount int, estimatedDuration time.Duration, rounds int, intervalMs int) error {
	if !canSendEmail() {
		return nil
	}

	now := timeutil.Now()
	subject := fmt.Sprintf("【测试开始】pipetGo - %s - %s", email.GetDeviceName(), timeutil.FormatDateTime(now))

	var body strings.Builder
	body.WriteString("===== 测试开始通知 =====\n\n")
	body.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(now)))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", email.GetDeviceName()))
	body.WriteString(fmt.Sprintf("\n测试用例统计:\n"))
	body.WriteString(fmt.Sprintf("  本次测试用例总数: %d\n", testCaseCount))
	body.WriteString(fmt.Sprintf("  链式测试: %d\n", chainCount))
	body.WriteString(fmt.Sprintf("  独立测试: %d\n", independentCount))

	if rounds > 1 {
		body.WriteString(fmt.Sprintf("\n多轮测试配置:\n"))
		body.WriteString(fmt.Sprintf("  测试轮数: %d\n", rounds))
		body.WriteString(fmt.Sprintf("  轮间隔: %dms\n", intervalMs))
	}

	if estimatedDuration > 0 {
		totalDuration := estimatedDuration * time.Duration(rounds)
		if rounds > 1 {
			totalDuration += time.Duration((rounds-1)*intervalMs) * time.Millisecond
		}
		estimatedEndTime := now.Add(totalDuration)
		body.WriteString(fmt.Sprintf("\n预估执行时间: %v\n", totalDuration.Round(time.Millisecond)))
		body.WriteString(fmt.Sprintf("预测结束时间: %s\n", timeutil.FormatDateTime(estimatedEndTime)))
	} else {
		body.WriteString(fmt.Sprintf("\n预估执行时间: 无历史数据\n"))
		body.WriteString(fmt.Sprintf("预测结束时间: 无法预测\n"))
	}
	body.WriteString("\n===== 通知结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	logger.Info("开始发送测试开始通知邮件",
		zap.String("主题", subject),
		zap.Int("用例总数", testCaseCount))
	return email.SendEmail(subject, body.String())
}

func SendWeeklyReportEmail(consecutiveFailN int, topSlowN int) error {
	return SendWeeklyReportEmailWithTemplate(consecutiveFailN, topSlowN, DefaultWeekTemplate())
}

func SendWeeklyReportEmailWithTemplate(consecutiveFailN int, topSlowN int, tmpl ReportTemplate) error {
	logger.Debug("开始准备发送周报邮件",
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN),
		zap.Bool("模板显示汇总", tmpl.ShowSummary),
		zap.Bool("模板显示增长", tmpl.ShowGrowth),
		zap.Bool("模板显示错误率", tmpl.ShowError),
		zap.Bool("模板显示慢接口", tmpl.ShowSlow),
		zap.Bool("模板显示告警", tmpl.ShowAlert))

	if !canSendEmail() {
		logger.Warn("周报邮件未发送: 邮件发送功能未启用或配置不完整")
		return nil
	}

	logger.Debug("邮件配置检查通过，开始生成周报")
	report, err := GenerateWeekReportWithTemplate(email.GetDeviceName(), consecutiveFailN, topSlowN, tmpl)
	if err != nil {
		logger.Error("生成周报失败", zap.Error(err))
		return err
	}

	logger.Debug("周报生成完成",
		zap.String("周期", report.StartDate+" ~ "+report.EndDate),
		zap.Int("日汇总条数", len(report.DailyStats)),
		zap.Int("慢接口条数", len(report.SlowCases)),
		zap.Int("告警条数", len(report.AlertCases)))

	subject := fmt.Sprintf("【测试周报】pipetGo - %s (%s ~ %s)", email.GetDeviceName(), report.StartDate, report.EndDate)
	body := report.FormatWeekReport()

	logger.Debug("周报格式化完成", zap.String("类型", "周报"), zap.Int("正文长度", len(body)))
	logger.Info("开始发送周报邮件",
		zap.String("主题", subject))
	err = email.SendEmail(subject, body)
	if err != nil {
		logger.Error("发送周报邮件失败", zap.Error(err))
	} else {
		logger.Info("周报邮件发送成功", zap.String("主题", subject))
	}
	return err
}

func SendDailyReportEmail(consecutiveFailN int, topSlowN int) error {
	return SendDailyReportEmailWithTemplate(consecutiveFailN, topSlowN, DefaultDayTemplate())
}

func SendDailyReportEmailWithTemplate(consecutiveFailN int, topSlowN int, tmpl ReportTemplate) error {
	logger.Debug("开始准备发送日报邮件",
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN),
		zap.Bool("模板显示汇总", tmpl.ShowSummary),
		zap.Bool("模板显示增长", tmpl.ShowGrowth),
		zap.Bool("模板显示错误率", tmpl.ShowError),
		zap.Bool("模板显示慢接口", tmpl.ShowSlow),
		zap.Bool("模板显示告警", tmpl.ShowAlert))

	if !canSendEmail() {
		logger.Warn("日报邮件未发送: 邮件发送功能未启用或配置不完整")
		return nil
	}

	logger.Debug("邮件配置检查通过，开始生成日报")
	report, err := GenerateDayReportWithTemplate(email.GetDeviceName(), consecutiveFailN, topSlowN, tmpl)
	if err != nil {
		logger.Error("生成日报失败", zap.Error(err))
		return err
	}

	logger.Debug("日报生成完成",
		zap.String("日期", report.Date),
		zap.Int("当日汇总条数", len(report.DailyStats)),
		zap.Int("趋势数据条数", len(report.TrendStats)),
		zap.Int("慢接口条数", len(report.SlowCases)),
		zap.Int("告警条数", len(report.AlertCases)))

	subject := fmt.Sprintf("【测试日报】pipetGo - %s (%s)", email.GetDeviceName(), report.Date)
	body := report.FormatDayReport()

	logger.Debug("日报格式化完成", zap.String("类型", "日报"), zap.Int("连续失败阈值", consecutiveFailN), zap.Int("慢接口上限", topSlowN), zap.Int("正文长度", len(body)))
	logger.Info("开始发送日报邮件",
		zap.String("主题", subject))
	err = email.SendEmail(subject, body)
	if err != nil {
		logger.Error("发送日报邮件失败", zap.Error(err))
	} else {
		logger.Info("日报邮件发送成功", zap.String("主题", subject))
	}
	return err
}

func SendMonthlyReportEmail(consecutiveFailN int, topSlowN int) error {
	return SendMonthlyReportEmailWithTemplate(consecutiveFailN, topSlowN, DefaultMonthTemplate())
}

func SendMonthlyReportEmailWithTemplate(consecutiveFailN int, topSlowN int, tmpl ReportTemplate) error {
	logger.Debug("开始准备发送月报邮件",
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN),
		zap.Bool("模板显示汇总", tmpl.ShowSummary),
		zap.Bool("模板显示增长", tmpl.ShowGrowth),
		zap.Bool("模板显示错误率", tmpl.ShowError),
		zap.Bool("模板显示慢接口", tmpl.ShowSlow),
		zap.Bool("模板显示告警", tmpl.ShowAlert))

	if !canSendEmail() {
		logger.Warn("月报邮件未发送: 邮件发送功能未启用或配置不完整")
		return nil
	}

	logger.Debug("邮件配置检查通过，开始生成月报")
	report, err := GenerateMonthReportWithTemplate(email.GetDeviceName(), consecutiveFailN, topSlowN, tmpl)
	if err != nil {
		logger.Error("生成月报失败", zap.Error(err))
		return err
	}

	logger.Debug("月报生成完成",
		zap.String("周期", report.StartDate+" ~ "+report.EndDate),
		zap.Int("日汇总条数", len(report.DailyStats)),
		zap.Int("慢接口条数", len(report.SlowCases)),
		zap.Int("告警条数", len(report.AlertCases)))

	subject := fmt.Sprintf("【测试月报】pipetGo - %s (%s ~ %s)", email.GetDeviceName(), report.StartDate, report.EndDate)
	body := report.FormatMonthReport()

	logger.Debug("月报格式化完成", zap.String("类型", "月报"), zap.Int("正文长度", len(body)))
	logger.Info("开始发送月报邮件",
		zap.String("主题", subject))
	err = email.SendEmail(subject, body)
	if err != nil {
		logger.Error("发送月报邮件失败", zap.Error(err))
	} else {
		logger.Info("月报邮件发送成功", zap.String("主题", subject))
	}
	return err
}

func SendYearlyReportEmail(consecutiveFailN int, topSlowN int) error {
	return SendYearlyReportEmailWithTemplate(consecutiveFailN, topSlowN, DefaultYearTemplate())
}

func SendYearlyReportEmailWithTemplate(consecutiveFailN int, topSlowN int, tmpl ReportTemplate) error {
	logger.Debug("开始准备发送年报邮件",
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN),
		zap.Bool("模板显示汇总", tmpl.ShowSummary),
		zap.Bool("模板显示增长", tmpl.ShowGrowth),
		zap.Bool("模板显示错误率", tmpl.ShowError),
		zap.Bool("模板显示慢接口", tmpl.ShowSlow),
		zap.Bool("模板显示告警", tmpl.ShowAlert))

	if !canSendEmail() {
		logger.Warn("年报邮件未发送: 邮件发送功能未启用或配置不完整")
		return nil
	}

	logger.Debug("邮件配置检查通过，开始生成年报")
	report, err := GenerateYearReportWithTemplate(email.GetDeviceName(), consecutiveFailN, topSlowN, tmpl)
	if err != nil {
		logger.Error("生成年报失败", zap.Error(err))
		return err
	}

	logger.Debug("年报生成完成",
		zap.String("周期", report.StartDate+" ~ "+report.EndDate),
		zap.Int("日汇总条数", len(report.DailyStats)),
		zap.Int("慢接口条数", len(report.SlowCases)),
		zap.Int("告警条数", len(report.AlertCases)))

	subject := fmt.Sprintf("【测试年报】pipetGo - %s (%s ~ %s)", email.GetDeviceName(), report.StartDate, report.EndDate)
	body := report.FormatYearReport()

	logger.Debug("年报格式化完成", zap.String("类型", "年报"), zap.Int("正文长度", len(body)))
	logger.Info("开始发送年报邮件",
		zap.String("主题", subject))
	err = email.SendEmail(subject, body)
	if err != nil {
		logger.Error("发送年报邮件失败", zap.Error(err))
	} else {
		logger.Info("年报邮件发送成功", zap.String("主题", subject))
	}
	return err
}

func SendTestReportEmailWithAlerts(results []testcase.TestResult, consecutiveFailN int, topSlowN int) error {
	if !canSendEmail() {
		return nil
	}

	logger.Info("准备发送测试报告邮件",
		zap.Int("结果数", len(results)),
		zap.Int("连续失败阈值", consecutiveFailN),
		zap.Int("慢接口上限", topSlowN))

	alerts, err := storage.GetConsecutiveFailures(consecutiveFailN)
	if err != nil {
		logger.Warn("获取连续失败告警失败，邮件报告可能缺少告警信息",
			zap.Int("连续失败阈值", consecutiveFailN),
			zap.Error(err))
	}
	alertIDs := make(map[string]bool)
	for _, a := range alerts {
		alertIDs[a.TestCaseID] = true
	}

	slowCases, err := storage.GetCaseAverageDurations("desc")
	if err != nil {
		logger.Warn("获取慢接口排行失败，邮件报告可能缺少慢接口数据", zap.Error(err))
	}
	if topSlowN <= 0 {
		topSlowN = 10
	}
	if len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	subject := fmt.Sprintf("【测试报告】pipetGo - %s - %s", email.GetDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString(fmt.Sprintf("===== 测试报告 =====\n\n"))
	body.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", email.GetDeviceName()))

	chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped, totalDuration := testcase.SummarizeResultsByType(results)
	totalPassed := chainPassed + independentPassed
	totalFailed := chainFailed + independentFailed
	totalSkipped := chainSkipped + independentSkipped

	passRate := 0.0
	if totalPassed+totalFailed > 0 {
		passRate = float64(totalPassed) / float64(totalPassed+totalFailed) * 100
	}

	body.WriteString(fmt.Sprintf("测试统计:\n"))
	body.WriteString(fmt.Sprintf("  总测试数: %d\n", totalPassed+totalFailed+totalSkipped))
	body.WriteString(fmt.Sprintf("  通过数:   %d\n", totalPassed))
	body.WriteString(fmt.Sprintf("  失败数:   %d\n", totalFailed))
	body.WriteString(fmt.Sprintf("  跳过数:   %d\n", totalSkipped))
	body.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", passRate))
	body.WriteString(fmt.Sprintf("  总耗时:   %v\n\n", totalDuration))

	body.WriteString(fmt.Sprintf("单例测试统计:\n"))
	body.WriteString(fmt.Sprintf("  测试数:   %d\n", independentPassed+independentFailed+independentSkipped))
	body.WriteString(fmt.Sprintf("  通过数:   %d\n", independentPassed))
	body.WriteString(fmt.Sprintf("  失败数:   %d\n", independentFailed))
	body.WriteString(fmt.Sprintf("  跳过数:   %d\n", independentSkipped))
	if independentPassed+independentFailed > 0 {
		body.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(independentPassed)/float64(independentPassed+independentFailed)*100))
	}
	body.WriteString("\n")

	body.WriteString(fmt.Sprintf("链式测试统计:\n"))
	body.WriteString(fmt.Sprintf("  测试数:   %d\n", chainPassed+chainFailed+chainSkipped))
	body.WriteString(fmt.Sprintf("  通过数:   %d\n", chainPassed))
	body.WriteString(fmt.Sprintf("  失败数:   %d\n", chainFailed))
	body.WriteString(fmt.Sprintf("  跳过数:   %d\n", chainSkipped))
	if chainPassed+chainFailed > 0 {
		body.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(chainPassed)/float64(chainPassed+chainFailed)*100))
	}
	body.WriteString("\n")

	aggregated := testcase.AggregateResultsByFile(results)
	hasFailures := false
	for _, r := range aggregated {
		if !r.Passed && !r.TestCase.Skip {
			hasFailures = true
			break
		}
	}
	if hasFailures {
		body.WriteString("失败详情:\n")
		body.WriteString(renderFailureTable(aggregated, true, alertIDs))
	}

	// 连续失败告警：标题含连续轮数，表格交由 renderAlertCases 动态渲染（不截断、列宽自适应）
	if len(alerts) > 0 {
		body.WriteString(fmt.Sprintf("\n⚠️ 连续失败告警 (连续%d轮):\n", consecutiveFailN))
		body.WriteString(renderAlertCases(alerts))
	}

	if len(slowCases) > 0 {
		body.WriteString(fmt.Sprintf("\n最慢接口 TOP %d:\n", len(slowCases)))
		body.WriteString(FormatCaseDurationTable(slowCases))
	}

	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	logger.Info("开始发送测试报告邮件",
		zap.String("主题", subject),
		zap.Int("总数", totalPassed+totalFailed+totalSkipped),
		zap.Int("通过", totalPassed),
		zap.Int("失败", totalFailed),
		zap.Float64("通过率", passRate))
	return email.SendEmail(subject, body.String())
}