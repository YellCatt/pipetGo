// Package cleaner 提供日志和测试报告的自动清理功能
package cleaner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"pipetGo/internal/logger"
	"pipetGo/internal/timeutil"
)

// Config 清理配置（与 config.go 中的 CleanupConfig 保持一致）
type Config struct {
	Enabled         bool     `mapstructure:"enabled"`          // 是否启用自动清理
	RetentionDays   int      `mapstructure:"retention_days"`   // 文件保留天数
	LogDir          string   `mapstructure:"log_dir"`          // 日志目录
	ReportDir       string   `mapstructure:"report_dir"`       // 测试报告目录
	DataDir         string   `mapstructure:"data_dir"`         // 数据目录
	IncludePatterns []string `mapstructure:"include_patterns"` // 要清理的文件模式列表
	ExcludePatterns []string `mapstructure:"exclude_patterns"` // 排除的文件模式列表
	IntervalHours   int      `mapstructure:"interval_hours"`   // 定时清理间隔（小时）
}

// Cleaner 清理器
type Cleaner struct {
	config   Config
	stopChan chan struct{}
	running  bool
}

// NewCleaner 创建清理器实例
func NewCleaner(config Config) *Cleaner {
	return &Cleaner{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start 启动定时清理任务
func (c *Cleaner) Start() error {
	if !c.config.Enabled {
		logger.Info("清理器已禁用，跳过启动")
		return nil
	}

	if c.running {
		return fmt.Errorf("cleaner is already running")
	}

	// 设置默认值
	c.setDefaults()

	c.running = true
	interval := time.Duration(c.config.IntervalHours) * time.Hour
	logger.Info("启动清理器",
		zap.Int("保留天数", c.config.RetentionDays),
		zap.String("日志目录", c.config.LogDir),
		zap.String("报告目录", c.config.ReportDir),
		zap.String("数据目录", c.config.DataDir),
		zap.Int("间隔小时", c.config.IntervalHours),
		zap.Any("包含模式", c.config.IncludePatterns),
		zap.Any("排除模式", c.config.ExcludePatterns))

	// 立即执行一次清理
	go c.cleanup()

	// 启动定时任务
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		logger.Debug("清理器定时循环已启动", zap.Duration("间隔", interval))
		for {
			select {
			case t := <-ticker.C:
				logger.Debug("清理器定时触发", zap.Time("时间", t))
				c.cleanup()
			case <-c.stopChan:
				logger.Info("清理器已停止")
				return
			}
		}
	}()

	return nil
}

// Stop 停止清理任务
func (c *Cleaner) Stop() {
	if !c.running {
		return
	}
	logger.Debug("停止清理器")
	c.running = false
	close(c.stopChan)
}

// Cleanup 执行一次清理（手动调用）
func (c *Cleaner) Cleanup() error {
	if !c.config.Enabled {
		return fmt.Errorf("cleaner is disabled")
	}
	c.setDefaults()
	return c.cleanup()
}

// setDefaults 设置默认值
func (c *Cleaner) setDefaults() {
	if c.config.RetentionDays <= 0 {
		c.config.RetentionDays = 30 // 默认保留30天
	}
	if c.config.IntervalHours <= 0 {
		c.config.IntervalHours = 24 // 默认每天清理一次
	}
	if len(c.config.IncludePatterns) == 0 {
		c.config.IncludePatterns = []string{"*.log", "*.json", "*.csv", "*.txt"}
	}
}

// cleanup 执行实际的清理操作
func (c *Cleaner) cleanup() error {
	logger.Info("开始执行清理任务")
	threshold := timeutil.Now().Add(-time.Duration(c.config.RetentionDays) * 24 * time.Hour)
	logger.Debug("清理阈值计算",
		zap.Int("保留天数", c.config.RetentionDays),
		zap.Time("阈值时间", threshold))

	totalDeleted := 0

	// 清理日志目录
	if c.config.LogDir != "" {
		logger.Debug("清理日志目录", zap.String("路径", c.config.LogDir))
		count, err := c.cleanupDirectory(c.config.LogDir, threshold)
		if err != nil {
			logger.Error("清理日志目录失败", zap.String("目录", c.config.LogDir), zap.Error(err))
		} else {
			logger.Debug("日志目录清理完成", zap.Int("删除数", count))
			totalDeleted += count
		}
	}

	// 清理报告目录
	if c.config.ReportDir != "" {
		logger.Debug("清理报告目录", zap.String("路径", c.config.ReportDir))
		count, err := c.cleanupDirectory(c.config.ReportDir, threshold)
		if err != nil {
			logger.Error("清理报告目录失败", zap.String("目录", c.config.ReportDir), zap.Error(err))
		} else {
			logger.Debug("报告目录清理完成", zap.Int("删除数", count))
			totalDeleted += count
		}
	}

	// 清理数据目录
	if c.config.DataDir != "" {
		logger.Debug("清理数据目录", zap.String("路径", c.config.DataDir))
		count, err := c.cleanupDirectory(c.config.DataDir, threshold)
		if err != nil {
			logger.Error("清理数据目录失败", zap.String("目录", c.config.DataDir), zap.Error(err))
		} else {
			logger.Debug("数据目录清理完成", zap.Int("删除数", count))
			totalDeleted += count
		}
	}

	logger.Info("清理任务完成", zap.Int("删除文件数", totalDeleted))
	return nil
}

// cleanupDirectory 清理指定目录中超过阈值时间的文件
func (c *Cleaner) cleanupDirectory(dir string, threshold time.Time) (int, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Debug("目录不存在，跳过", zap.String("目录", dir))
		return 0, nil
	}

	count := 0
	scanned := 0
	matchedInclude := 0
	var err error
	err = filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			logger.Warn("遍历清理目录出错", zap.String("路径", path), zap.Error(walkErr))
			return walkErr
		}

		if info.IsDir() {
			return nil
		}

		scanned++

		// 检查文件模式过滤
		if !c.matchesIncludePatterns(path) {
			return nil
		}
		matchedInclude++

		if c.matchesExcludePatterns(path) {
			logger.Debug("文件匹配排除模式，跳过", zap.String("路径", path))
			return nil
		}

		// 检查文件修改时间
		if info.ModTime().Before(threshold) {
			if rmErr := os.Remove(path); rmErr != nil {
				logger.Warn("删除文件失败", zap.String("路径", path), zap.Error(rmErr))
				return rmErr
			}
			count++
			logger.Info("已删除旧文件", zap.String("路径", path), zap.Time("修改时间", info.ModTime()), zap.Int64("大小", info.Size()))
		}

		return nil
	})

	logger.Debug("目录扫描完成",
		zap.String("目录", dir),
		zap.Int("扫描文件数", scanned),
		zap.Int("匹配包含模式数", matchedInclude),
		zap.Int("删除数", count))
	return count, err
}

// matchesIncludePatterns 检查文件是否匹配包含模式
func (c *Cleaner) matchesIncludePatterns(path string) bool {
	if len(c.config.IncludePatterns) == 0 {
		return true
	}

	baseName := filepath.Base(path)
	for _, pattern := range c.config.IncludePatterns {
		matched, err := filepath.Match(pattern, baseName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// matchesExcludePatterns 检查文件是否匹配排除模式
func (c *Cleaner) matchesExcludePatterns(path string) bool {
	if len(c.config.ExcludePatterns) == 0 {
		return false
	}

	baseName := filepath.Base(path)
	for _, pattern := range c.config.ExcludePatterns {
		matched, err := filepath.Match(pattern, baseName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// GetConfig 获取当前配置
func (c *Cleaner) GetConfig() Config {
	return c.config
}

// IsRunning 检查清理器是否正在运行
func (c *Cleaner) IsRunning() bool {
	return c.running
}

// ExtractLogDir 从日志输出路径中提取目录路径
// 例如："./logs/pipet.log" -> "./logs"
func ExtractLogDir(logOutputPath string) string {
	if logOutputPath == "" || logOutputPath == "stdout" || logOutputPath == "stderr" {
		return ""
	}
	return filepath.Dir(logOutputPath)
}

// CompressResponseBody 压缩接口返回内容
// 去除换行符、制表符和多余的空格，对于 JSON 格式进行智能压缩
// body: 原始响应体
// 返回: 压缩后的响应体
func CompressResponseBody(body string) string {
	if body == "" {
		return ""
	}

	compressed := strings.ReplaceAll(body, "\n", "")
	compressed = strings.ReplaceAll(compressed, "\r", "")
	compressed = strings.ReplaceAll(compressed, "\t", "")

	spaceRe := regexp.MustCompile(`\s+`)
	compressed = spaceRe.ReplaceAllString(compressed, " ")

	compressed = strings.TrimSpace(compressed)

	if isJSON(compressed) {
		compressed = compressJSON(compressed)
	}

	return compressed
}

// isJSON 检查字符串是否为 JSON 格式
func isJSON(s string) bool {
	var js json.RawMessage
	return json.Unmarshal([]byte(s), &js) == nil
}

// compressJSON 压缩 JSON 字符串（去除所有空格）
func compressJSON(jsonStr string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return jsonStr
	}
	compressed, err := json.Marshal(data)
	if err != nil {
		return jsonStr
	}
	return string(compressed)
}