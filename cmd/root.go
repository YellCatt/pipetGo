package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"pipet/config"
	"pipet/internal/httpclient"
	"pipet/internal/logger"
	"pipet/internal/psv"
	"pipet/internal/testcase"
	"pipet/internal/vars"
)

var (
	rootCmd = &cobra.Command{
		Use:   "pipet [paths...]",
		Short: "pipet - API Testing Tool",
		Long:  `A powerful enterprise-grade API testing tool written in Go.`,
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runTests(args)
		},
	}

	tagsFlag string
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.Error("Failed to execute command", zap.Error(err))
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.Flags().StringVar(&config.CfgFile, "config", "", "config file (default is ./config/config.yaml)")
	rootCmd.Flags().StringVarP(&tagsFlag, "tags", "t", "", "filter tests by tags (comma-separated)")
}

func initConfig() {
	config.InitConfig()
	logger.InitLogger(logger.LogConfig{
		Level:    config.AppConfig.Log.Level,
		Encoding: config.AppConfig.Log.Encoding,
		Output:   config.AppConfig.Log.Output,
	})
	vars.Set("base_url", config.AppConfig.Target.BaseURL)
}

func runTests(paths []string) {
	httpclient.InitClient()

	if len(paths) == 0 {
		paths = []string{config.AppConfig.Test.TestCaseDir}
	}

	testCases, err := psv.ParseFiles(paths)
	if err != nil {
		logger.Error("Failed to parse PSV files", zap.Error(err))
		os.Exit(1)
	}

	var tags []string
	if tagsFlag != "" {
		tags = strings.Split(tagsFlag, ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	testCases = testcase.FilterByTags(testCases, tags)

	if len(testCases) == 0 {
		logger.Info("No test cases to run")
		return
	}

	logger.Info("Starting API tests", zap.Int("count", len(testCases)))

	results := testcase.RunParallel(testCases)

	testcase.PrintSummary(results)

	allReport, errorReport := testcase.GenerateReport(results)
	allPath, errorPath := testcase.SaveReports(allReport, errorReport)

	fmt.Printf("\nPSV 报告已保存: %s\n", allPath)
	if errorPath != "" {
		fmt.Printf("异常用例 PSV 报告已保存: %s\n", errorPath)
	}

	failedCount := 0
	for _, r := range results {
		if !r.Passed && !r.TestCase.Skip {
			failedCount++
		}
	}

	if failedCount > 0 {
		os.Exit(1)
	}
}