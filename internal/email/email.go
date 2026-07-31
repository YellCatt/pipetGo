// Package email 提供邮件发送功能
package email

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
	"time"

	"pipetGo/internal/reporting"
	"pipetGo/internal/storage"
	"pipetGo/internal/testcase"
	"pipetGo/internal/timeutil"
)

type EmailConfig struct {
	Enabled    bool
	FromEmail  string
	ToEmail    []string
	AuthCode   string
	SMTPServer string
	SMTPPort   int
	DeviceName string
}

var Config EmailConfig

func InitEmail(cfg EmailConfig) {
	Config = cfg
}

// getDeviceName 获取设备名称，优先使用配置值，未配置时自动获取主机名
func getDeviceName() string {
	if Config.DeviceName != "" {
		return Config.DeviceName
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "未知设备"
	}
	return hostname
}

func formatSubject(subject string) string {
	return subject
}

func formatBody(body string) string {
	return body
}

func SendEmail(subject, body string) error {
	subject = formatSubject(subject)
	body = formatBody(body)

	toEmails := strings.Join(Config.ToEmail, ", ")
	msg := []byte("From: " + Config.FromEmail + "\r\n" +
		"To: " + toEmails + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", Config.SMTPServer, Config.SMTPPort)
	auth := smtp.PlainAuth("", Config.FromEmail, Config.AuthCode, Config.SMTPServer)

	log.Printf("连接 SMTP 服务器: %s\n", addr)

	// 使用 TLS 连接
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         Config.SMTPServer,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("TLS 连接失败: %v\n", err)
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, Config.SMTPServer)
	if err != nil {
		log.Printf("创建 SMTP 客户端失败: %v\n", err)
		return err
	}
	defer client.Close()

	// 认证
	if err := client.Auth(auth); err != nil {
		log.Printf("SMTP 认证失败: %v\n", err)
		return err
	}

	// 设置发件人
	if err := client.Mail(Config.FromEmail); err != nil {
		log.Printf("设置发件人失败: %v\n", err)
		return err
	}

	// 设置多个收件人
	for _, to := range Config.ToEmail {
		if err := client.Rcpt(to); err != nil {
			log.Printf("设置收件人 %s 失败: %v\n", to, err)
			return err
		}
	}

	// 发送邮件内容
	w, err := client.Data()
	if err != nil {
		log.Printf("获取数据写入器失败: %v\n", err)
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		log.Printf("写入邮件内容失败: %v\n", err)
		return err
	}

	err = w.Close()
	if err != nil {
		log.Printf("关闭数据写入器失败: %v\n", err)
		return err
	}

	log.Println("✅ 邮件发送成功")
	return nil
}

func GenerateTestReportContent(results []testcase.TestResult) string {
	var sb strings.Builder

	// 使用东八区时间
	now := timeutil.Now()

	sb.WriteString(fmt.Sprintf("===== 测试报告 =====\n\n"))
	sb.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(now)))
	sb.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))

	var totalDuration time.Duration

	// 使用统一的统计函数，与测试开始邮件口径一致
	chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped, totalDuration := testcase.SummarizeResultsByType(results)

	totalPassed := chainPassed + independentPassed
	totalFailed := chainFailed + independentFailed
	totalSkipped := chainSkipped + independentSkipped

	// 聚合结果用于失败详情展示
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
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")
		sb.WriteString(fmt.Sprintf("%-15s %-40s %-10s %-15s %s\n", "ID", "描述", "状态", "耗时", "错误信息"))
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")

		for _, r := range aggregated {
			if !r.Passed && !r.TestCase.Skip {
				sb.WriteString(fmt.Sprintf("%-15s %-40s %-10s %-15v %s\n",
					r.TestCase.ID,
					r.TestCase.Desc,
					"F",
					r.Duration,
					r.Error))
			}
		}
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")
	}

	sb.WriteString("\n===== 报告结束 =====\n")
	sb.WriteString("来自 pipetGo 测试程序")

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func SendTestReportEmail(results []testcase.TestResult) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	// 使用东八区时间
	subject := fmt.Sprintf("【测试报告】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	body := GenerateTestReportContent(results)

	log.Println("发送测试报告邮件...")
	return SendEmail(subject, body)
}

// SendErrorReportEmail 发送异常退出报告邮件
func SendErrorReportEmail(errorMessage string) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	subject := fmt.Sprintf("【测试异常】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var body strings.Builder
	body.WriteString("===== 测试异常报告 =====\n\n")
	body.WriteString(fmt.Sprintf("发生时间: %s\n", timeutil.FormatDateTime(timeutil.Now())))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))
	body.WriteString(fmt.Sprintf("\n异常信息:\n"))
	body.WriteString(fmt.Sprintf("  %s\n", errorMessage))
	body.WriteString("\n===== 报告结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	log.Println("发送异常报告邮件...")
	return SendEmail(subject, body.String())
}

// SendTestStartEmail 发送测试开始通知邮件
func SendTestStartEmail(testCaseCount, chainCount, independentCount int, estimatedDuration time.Duration, rounds int, intervalMs int) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	// 使用东八区时间
	now := timeutil.Now()
	subject := fmt.Sprintf("【测试开始】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(now))

	var body strings.Builder
	body.WriteString("===== 测试开始通知 =====\n\n")
	body.WriteString(fmt.Sprintf("执行时间: %s\n", timeutil.FormatDateTime(now)))
	body.WriteString(fmt.Sprintf("测试设备: %s\n", getDeviceName()))
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
		// 计算多轮测试的总预估时间
		totalDuration := estimatedDuration * time.Duration(rounds)
		if rounds > 1 {
			// 添加轮间隔时间
			totalDuration += time.Duration((rounds-1)*intervalMs) * time.Millisecond
		}
		estimatedEndTime := now.Add(totalDuration)
		body.WriteString(fmt.Sprintf("\n预估执行时间: %v\n", totalDuration))
		body.WriteString(fmt.Sprintf("预测结束时间: %s\n", timeutil.FormatDateTime(estimatedEndTime)))
	} else {
		body.WriteString(fmt.Sprintf("\n预估执行时间: 无历史数据\n"))
		body.WriteString(fmt.Sprintf("预测结束时间: 无法预测\n"))
	}
	body.WriteString("\n===== 通知结束 =====\n")
	body.WriteString("来自 pipetGo 测试程序")

	log.Println("发送测试开始通知邮件...")
	return SendEmail(subject, body.String())
}

// SendHTMLEmail 发送HTML格式邮件
func SendHTMLEmail(subject, htmlBody string) error {
	subject = formatSubject(subject)

	toEmails := strings.Join(Config.ToEmail, ", ")
	msg := []byte("From: " + Config.FromEmail + "\r\n" +
		"To: " + toEmails + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		htmlBody + "\r\n")

	addr := fmt.Sprintf("%s:%d", Config.SMTPServer, Config.SMTPPort)
	auth := smtp.PlainAuth("", Config.FromEmail, Config.AuthCode, Config.SMTPServer)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         Config.SMTPServer,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("TLS 连接失败: %v\n", err)
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, Config.SMTPServer)
	if err != nil {
		log.Printf("创建 SMTP 客户端失败: %v\n", err)
		return err
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		log.Printf("SMTP 认证失败: %v\n", err)
		return err
	}

	if err := client.Mail(Config.FromEmail); err != nil {
		log.Printf("设置发件人失败: %v\n", err)
		return err
	}

	for _, to := range Config.ToEmail {
		if err := client.Rcpt(to); err != nil {
			log.Printf("设置收件人 %s 失败: %v\n", to, err)
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		log.Printf("获取数据写入器失败: %v\n", err)
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		log.Printf("写入邮件内容失败: %v\n", err)
		return err
	}

	err = w.Close()
	if err != nil {
		log.Printf("关闭数据写入器失败: %v\n", err)
		return err
	}

	log.Println("HTML 邮件发送成功")
	return nil
}

// SendWeeklyReportEmail 发送周报邮件
func SendWeeklyReportEmail(consecutiveFailN int, topSlowN int) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过周报邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过周报邮件发送")
		return nil
	}

	report, err := reporting.GenerateWeekReport(getDeviceName(), consecutiveFailN, topSlowN)
	if err != nil {
		log.Printf("生成周报失败: %v\n", err)
		return err
	}

	subject := fmt.Sprintf("【测试周报】pipetGo - %s (%s ~ %s)", getDeviceName(), report.StartDate, report.EndDate)
	htmlBody := report.FormatWeekReportHTML()

	log.Println("发送测试周报邮件...")
	return SendHTMLEmail(subject, htmlBody)
}

// SendMonthlyReportEmail 发送月报邮件
func SendMonthlyReportEmail(consecutiveFailN int, topSlowN int) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过月报邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过月报邮件发送")
		return nil
	}

	report, err := reporting.GenerateMonthReport(getDeviceName(), consecutiveFailN, topSlowN)
	if err != nil {
		log.Printf("生成月报失败: %v\n", err)
		return err
	}

	subject := fmt.Sprintf("【测试月报】pipetGo - %s (%s ~ %s)", getDeviceName(), report.StartDate, report.EndDate)
	htmlBody := report.FormatMonthReportHTML()

	log.Println("发送测试月报邮件...")
	return SendHTMLEmail(subject, htmlBody)
}

// SendYearlyReportEmail 发送年报邮件
func SendYearlyReportEmail(consecutiveFailN int, topSlowN int) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过年报邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过年报邮件发送")
		return nil
	}

	report, err := reporting.GenerateYearReport(getDeviceName(), consecutiveFailN, topSlowN)
	if err != nil {
		log.Printf("生成年报失败: %v\n", err)
		return err
	}

	subject := fmt.Sprintf("【测试年报】pipetGo - %s (%s ~ %s)", getDeviceName(), report.StartDate, report.EndDate)
	htmlBody := report.FormatYearReportHTML()

	log.Println("发送测试年报邮件...")
	return SendHTMLEmail(subject, htmlBody)
}

// SendTestReportEmailWithAlerts 发送测试报告邮件，包含连续失败标红告警
func SendTestReportEmailWithAlerts(results []testcase.TestResult, consecutiveFailN int, topSlowN int) error {
	if !Config.Enabled {
		log.Println("邮件发送功能已禁用，跳过邮件发送")
		return nil
	}
	if Config.FromEmail == "" || len(Config.ToEmail) == 0 || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	alerts, _ := storage.GetConsecutiveFailures(consecutiveFailN)
	alertIDs := make(map[string]bool)
	for _, a := range alerts {
		alertIDs[a.TestCaseID] = true
	}

	slowCases, _ := storage.GetCaseAverageDurations("desc")
	if topSlowN > 0 && len(slowCases) > topSlowN {
		slowCases = slowCases[:topSlowN]
	}

	subject := fmt.Sprintf("【测试报告】pipetGo - %s - %s", getDeviceName(), timeutil.FormatDateTime(timeutil.Now()))

	var htmlBody strings.Builder
	htmlBody.WriteString(`<html><body style="font-family: monospace; font-size: 13px; background: #1a1a2e; color: #e0e0e0; padding: 20px;">`)
	htmlBody.WriteString(fmt.Sprintf(`<h2 style="color: #00d4ff;">测试报告</h2>`))
	htmlBody.WriteString(fmt.Sprintf(`<p>执行时间: %s | 设备: <b>%s</b></p>`,
		timeutil.FormatDateTime(timeutil.Now()), getDeviceName()))

	// 统计
	chainPassed, chainFailed, chainSkipped, independentPassed, independentFailed, independentSkipped, totalDuration := testcase.SummarizeResultsByType(results)
	totalPassed := chainPassed + independentPassed
	totalFailed := chainFailed + independentFailed
	totalSkipped := chainSkipped + independentSkipped

	passRate := 0.0
	if totalPassed+totalFailed > 0 {
		passRate = float64(totalPassed) / float64(totalPassed+totalFailed) * 100
	}

	htmlBody.WriteString(`<h3>测试统计</h3>`)
	htmlBody.WriteString(`<table style="border-collapse: collapse; color: #e0e0e0;">`)
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>总测试数:</td><td>%d</td></tr>`, totalPassed+totalFailed+totalSkipped))
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>通过数:</td><td style="color: #00ff88;">%d</td></tr>`, totalPassed))
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>失败数:</td><td style="color: #ff4444;">%d</td></tr>`, totalFailed))
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>跳过数:</td><td>%d</td></tr>`, totalSkipped))
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>通过率:</td><td>%.2f%%</td></tr>`, passRate))
	htmlBody.WriteString(fmt.Sprintf(`<tr><td>总耗时:</td><td>%v</td></tr>`, totalDuration))
	htmlBody.WriteString(`</table>`)

	// 失败详情（连续失败标红）
	aggregated := testcase.AggregateResultsByFile(results)
	hasFailures := false
	for _, r := range aggregated {
		if !r.Passed && !r.TestCase.Skip {
			hasFailures = true
			break
		}
	}
	if hasFailures {
		htmlBody.WriteString(`<h3>失败详情</h3>`)
		htmlBody.WriteString(`<table style="border-collapse: collapse; width: 100%%; color: #e0e0e0;">`)
		htmlBody.WriteString(`<tr style="background: #1a1a3e;"><th>用例ID</th><th>描述</th><th>状态</th><th>耗时</th><th>错误信息</th></tr>`)
		for _, r := range aggregated {
			if !r.Passed && !r.TestCase.Skip {
				rowStyle := ""
				if alertIDs[r.TestCase.ID] {
					rowStyle = ` style="background: #4a0000; color: #ff6666; font-weight: bold;"`
				}
				htmlBody.WriteString(fmt.Sprintf(`<tr%s><td>%s</td><td>%s</td><td style="color: #ff4444;">❌ FAIL</td><td>%v</td><td>%s</td></tr>`,
					rowStyle, r.TestCase.ID, r.TestCase.Desc, r.Duration, r.Error))
			}
		}
		htmlBody.WriteString(`</table>`)
	}

	// 连续失败告警
	if len(alerts) > 0 {
		htmlBody.WriteString(`<h3 style="color: #ff4444;">⚠️ 连续失败告警</h3>`)
		htmlBody.WriteString(`<p>以下用例连续 ` + fmt.Sprintf("%d", consecutiveFailN) + ` 轮失败，请重点关注:</p>`)
		htmlBody.WriteString(`<table style="border-collapse: collapse; color: #ff6666;">`)
		htmlBody.WriteString(`<tr style="background: #3a0000;"><th>用例ID</th><th>描述</th><th>最近执行时间</th></tr>`)
		for _, a := range alerts {
			htmlBody.WriteString(fmt.Sprintf(`<tr style="background: #2a0000;"><td>%s</td><td>%s</td><td>%s</td></tr>`,
				a.TestCaseID, a.TestCaseDesc, a.LastExecuted))
		}
		htmlBody.WriteString(`</table>`)
	}

	// 慢接口排名
	if len(slowCases) > 0 {
		htmlBody.WriteString(`<h3>最慢接口 TOP ` + fmt.Sprintf("%d", len(slowCases)) + `</h3>`)
		htmlBody.WriteString(`<pre style="background: #0d0d1a; padding: 10px; border-radius: 4px;">`)
		htmlBody.WriteString(reporting.FormatCaseDurationTable(slowCases))
		htmlBody.WriteString(`</pre>`)
	}

	htmlBody.WriteString(`<p style="color: #666;">来自 pipetGo 测试程序</p>`)
	htmlBody.WriteString(`</body></html>`)

	log.Println("发送HTML测试报告邮件...")
	return SendHTMLEmail(subject, htmlBody.String())
}
