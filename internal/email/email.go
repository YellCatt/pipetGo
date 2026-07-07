// Package email 提供邮件发送功能
package email

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"pipetGo/internal/testcase"
)

type EmailConfig struct {
	FromEmail  string
	ToEmail    string
	AuthCode   string
	SMTPServer string
	SMTPPort   int
}

var Config EmailConfig

func InitEmail(cfg EmailConfig) {
	Config = cfg
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

	msg := []byte("From: " + Config.FromEmail + "\r\n" +
		"To: " + Config.ToEmail + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body + "\r\n")

	addr := fmt.Sprintf("%s:%d", Config.SMTPServer, Config.SMTPPort)
	auth := smtp.PlainAuth("", Config.FromEmail, Config.AuthCode, Config.SMTPServer)

	log.Printf("连接 SMTP 服务器: %s\n", addr)
	err := smtp.SendMail(addr, auth, Config.FromEmail, []string{Config.ToEmail}, msg)
	if err != nil {
		log.Printf("邮件发送失败: %v\n", err)
		return err
	}

	log.Println("✅ 邮件发送成功")
	return nil
}

func GenerateTestReportContent(results []testcase.TestResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("===== 测试报告 =====\n\n"))
	sb.WriteString(fmt.Sprintf("执行时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	var passed, failed, skipped int
	var totalDuration time.Duration

	for _, r := range results {
		totalDuration += r.Duration
		if r.TestCase.Skip {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	sb.WriteString(fmt.Sprintf("测试统计:\n"))
	sb.WriteString(fmt.Sprintf("  总测试数: %d\n", passed+failed+skipped))
	sb.WriteString(fmt.Sprintf("  通过数:   %d\n", passed))
	sb.WriteString(fmt.Sprintf("  失败数:   %d\n", failed))
	sb.WriteString(fmt.Sprintf("  跳过数:   %d\n", skipped))
	sb.WriteString(fmt.Sprintf("  通过率:   %.2f%%\n", float64(passed)/float64(passed+failed)*100))
	sb.WriteString(fmt.Sprintf("  总耗时:   %v\n\n", totalDuration))

	if len(results) > 0 {
		sb.WriteString("测试详情:\n")
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")
		sb.WriteString(fmt.Sprintf("%-15s %-30s %-10s %-15s %s\n", "ID", "描述", "状态", "耗时", "错误信息"))
		sb.WriteString("-" + strings.Repeat("-", 78) + "\n")

		for _, r := range results {
			status := "✅ PASS"
			if r.TestCase.Skip {
				status = "⏭ SKIP"
			} else if !r.Passed {
				status = "❌ FAIL"
			}
			sb.WriteString(fmt.Sprintf("%-15s %-30s %-10s %-15v %s\n",
				r.TestCase.ID,
				truncateString(r.TestCase.Desc, 28),
				status,
				r.Duration,
				r.Error))
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
	if Config.FromEmail == "" || Config.ToEmail == "" || Config.AuthCode == "" {
		log.Println("邮件配置未设置，跳过邮件发送")
		return nil
	}

	subject := fmt.Sprintf("【测试报告】pipetGo - %s", time.Now().Format("2006-01-02 15:04"))
	body := GenerateTestReportContent(results)

	log.Println("发送测试报告邮件...")
	return SendEmail(subject, body)
}
