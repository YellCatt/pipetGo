// Package cmd 提供命令行接口功能
// 使用 cobra 框架实现命令行参数解析和子命令管理
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"pipetGo/config"
	"pipetGo/internal/email"
	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
	"pipetGo/internal/psv"
	"pipetGo/internal/testcase"
	"pipetGo/internal/vars"
)

var (
	// rootCmd 是命令行应用的根命令
	rootCmd = &cobra.Command{
		Use:   "pipet [paths...]",
		Short: "pipet - API Testing Tool",
		Long:  `A powerful enterprise-grade API testing tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runTests(args)
		},
	}

	// tagsFlag 存储命令行指定的标签过滤参数
	tagsFlag string
)

// Execute 启动命令行应用
// 调用 cobra 的 Execute 方法处理命令行输入
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("Failed to execute command", zap.Error(err))
		os.Exit(1)
	}
}

// init 函数在包初始化时执行
// 注册初始化函数和命令行参数
func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "filter tests by tags (comma-separated)")
}

// initConfig 初始化应用配置
// 依次初始化：配置、日志、全局变量、邮件配置
func initConfig() {
	config.InitConfig()
	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})
	vars.Set("base_url", config.AppConfig.Target.BaseURL)

	email.InitEmail(email.EmailConfig{
		FromEmail:  config.AppConfig.Email.From,
		ToEmail:    config.AppConfig.Email.To,
		AuthCode:   config.AppConfig.Email.AuthCode,
		SMTPServer: config.AppConfig.Email.SMTPServer,
		SMTPPort:   config.AppConfig.Email.SMTPPort,
	})
}

// runTests 执行测试主流程
// paths: 用户指定的测试用例路径列表
func runTests(paths []string) {
	// 初始化 HTTP 客户端
	httpclient.InitClient()

	// 如果未指定路径，使用默认测试用例目录
	if len(paths) == 0 {
		paths = []string{config.AppConfig.Test.TestCaseDir}
	}

	// 解析 PSV/CSV 测试用例文件
	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		os.Exit(1)
	}

	// 解析标签过滤参数
	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// 根据标签过滤测试用例
	testCases = testcase.FilterByTags(testCases, tags)

	// 如果没有测试用例，直接返回
	if len(testCases) == 0 {
		logger.Info("No test cases to run")
		return
	}

	// 开始执行测试
	logger.Info("Starting API tests", zap.Int("count", len(testCases)))

	// 运行测试（串行执行）
	results := testcase.RunParallel(testCases)

	// 打印测试摘要
	testcase.PrintSummary(results)

	// 生成并保存测试报告
	allReport, errorReport := testcase.GenerateReport(results)
	allPath, errorPath := testcase.SaveReports(allReport, errorReport)

	// 输出报告路径
	fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
	if errorPath != "" {
		fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
	}

	// 如果有失败的测试用例，退出码设为 1
	failedCount := 0
	for _, r := range results {
		if !r.Passed && !r.TestCase.Skip {
			failedCount++
		}
	}

	// 测试结束后发送邮件报告
	if err := email.SendTestReportEmail(results); err != nil {
		logger.Warn("Failed to send email report", zap.Error(err))
	}

	if failedCount > 0 {
		os.Exit(1)
	}
}
