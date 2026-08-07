package reporting

// StartupBanner 生成启动横幅（pipetGo 标题与设备信息）
func StartupBanner(deviceName, logFile string) string {
	data := BuildStartupHeaderData(deviceName, logFile)
	return "\n" + MustRenderText("startup_header", data) + "\n"
}

// TestStatsBanner 生成测试用例统计信息横幅
func TestStatsBanner(totalCount, chainCount, independentCount int, tags []string, executedCount, executedChainCount, executedIndependentCount int, estimatedDuration string, rounds int, intervalMs int) string {
	data := BuildStartupStatsData(totalCount, chainCount, independentCount, tags, executedCount, executedChainCount, executedIndependentCount, estimatedDuration, rounds, intervalMs)
	return "\n" + MustRenderText("startup_stats", data) + "\n"
}
