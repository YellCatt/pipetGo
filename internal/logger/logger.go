// Package logger 提供日志记录功能
// 使用 zap 日志库实现结构化日志，支持控制台和文件输出
// 文件输出采用日期 + 文件大小双重滚动策略：
//   - 按天区分日志，日期嵌入文件名（如 pipet_20260818_1.log）
//   - 单个日志文件上限 20MB，达到阈值后自动切分为同日期下的多个分片文件
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"pipetGo/internal/timeutil"
)

// log 是全局日志实例

var log *zap.Logger
var logLevel zap.AtomicLevel

// defaultMaxSize 默认单个日志文件上限：20MB
const defaultMaxSize = 20 * 1024 * 1024

// LogConfig 表示日志配置
type LogConfig struct {
	Level    string // 日志级别 (debug/info/warn/error)
	Encoding string // 日志格式 (json/console)
	Output   string // 输出位置 (stdout 或文件路径)
}

// InitLogger 初始化日志系统
// cfg: 日志配置
func InitLogger(cfg LogConfig) {
	logLevel = zap.NewAtomicLevelAt(getLogLevel(cfg.Level))

	Debug("初始化日志系统",
		zap.String("级别", cfg.Level),
		zap.String("编码", cfg.Encoding),
		zap.String("输出", cfg.Output))

	var encoderCfg zapcore.EncoderConfig

	switch cfg.Encoding {
	case "console":
		encoderCfg = zap.NewDevelopmentEncoderConfig()
	default:
		encoderCfg = zap.NewProductionEncoderConfig()
	}

	encoderCfg.TimeKey = "time"
	encoderCfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(timeutil.FormatDateTimeMs(t))
	}

	var encoder zapcore.Encoder
	switch cfg.Encoding {
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	default:
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	var core zapcore.Core

	if cfg.Output == "stdout" || cfg.Output == "" {
		core = zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), logLevel)
		Info("日志输出到 stdout")
	} else {
		writer, err := newRotateWriter(cfg.Output, defaultMaxSize)
		if err != nil {
			zap.L().Fatal("初始化日志滚动写入器失败", zap.Error(err))
			os.Exit(1)
		}
		core = zapcore.NewCore(encoder, writer, logLevel)
		Info("日志输出到文件",
			zap.String("路径", cfg.Output),
			zap.Int64("单文件上限MB", defaultMaxSize/(1024*1024)))
	}

	log = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(log)
	Info("日志系统初始化完成", zap.String("级别", cfg.Level))
}

// rotateWriter 实现日期 + 文件大小双重滚动
type rotateWriter struct {
	mu         sync.Mutex
	basePath   string
	maxSize    int64
	currentDay string
	currentIdx int
	file       *os.File
	fileSize   int64
}

// newRotateWriter 创建一个滚动写入器
// basePath 例如 "./logs/pipet.log"
// 实际输出: ./logs/pipet_20260818_1.log, pipet_20260818_2.log ...
func newRotateWriter(basePath string, maxSize int64) (*rotateWriter, error) {
	w := &rotateWriter{
		basePath: basePath,
		maxSize:  maxSize,
	}
	if err := w.rotate(timeutil.Now(), true); err != nil {
		return nil, err
	}
	return w, nil
}

// rotate 根据日期和大小滚动到目标文件
// 由调用方持有锁
func (w *rotateWriter) rotate(now time.Time, forceNewFile bool) error {
	day := timeutil.Format(now, "20060102")

	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	if day == w.currentDay && !forceNewFile {
		return nil
	}

	w.currentDay = day
	w.currentIdx = 1
	w.fileSize = 0
	return w.openFile(1)
}

// openFile 打开指定分片序号的文件
// 文件命名: {name}_{YYYYMMDD}_{idx}{ext}
// 例如: pipet_20260818_1.log, pipet_20260818_2.log
func (w *rotateWriter) openFile(idx int) error {
	dir := filepath.Dir(w.basePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := filepath.Base(w.basePath)
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	fullPath := filepath.Join(dir, fmt.Sprintf("%s_%s_%d%s", nameWithoutExt, w.currentDay, idx, ext))

	f, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	w.file = f
	w.fileSize = info.Size()
	w.currentIdx = idx
	fmt.Fprintf(os.Stderr, "[日志滚动器] 打开日志文件 路径=%s 大小=%d\n", fullPath, w.fileSize)
	return nil
}

// Write 实现 io.Writer，根据日期/大小检测是否需要滚动
func (w *rotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := timeutil.Now()
	day := timeutil.Format(now, "20060102")

	if day != w.currentDay {
		fmt.Fprintf(os.Stderr, "[日志滚动器] 日志日期切换触发滚动 旧日期=%s 新日期=%s\n", w.currentDay, day)
		if err := w.rotate(now, true); err != nil {
			return 0, err
		}
	}

	if w.fileSize+int64(len(p)) > w.maxSize {
		nextIdx := w.currentIdx + 1
		fmt.Fprintf(os.Stderr, "[日志滚动器] 日志文件达到大小上限，切换分片 当前分片=%d 新分片=%d 当前大小=%d 待写入=%d 上限=%d\n",
			w.currentIdx, nextIdx, w.fileSize, len(p), w.maxSize)
		if err := w.openFile(nextIdx); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.fileSize += int64(n)
	return n, err
}

// Sync 实现 zapcore.WriteSyncer
func (w *rotateWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

// addTimestampToFilename 保留用于兼容
func addTimestampToFilename(path string) string {
	dir := filepath.Dir(path)
	filename := filepath.Base(path)

	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)

	timestamp := timeutil.FormatCompact(timeutil.Now())

	return filepath.Join(dir, nameWithoutExt+"_"+timestamp+ext)
}

// ensureDir 确保日志目录存在
// path: 文件路径
func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
}

// getLogLevel 将字符串日志级别转换为 zapcore.Level
// level: 日志级别字符串
// 返回: zapcore.Level 枚举值
func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// UpdateLevel 动态更新日志级别（用于配置热加载）
func UpdateLevel(level string) {
	logLevel.SetLevel(getLogLevel(level))
	Info("日志级别已动态更新", zap.String("级别", level))
}

// l 返回当前 logger，若未初始化则回退到 zap 全局 logger
func l() *zap.Logger {
	if log != nil {
		return log
	}
	return zap.L()
}

// Debug 记录调试级别日志
func Debug(msg string, fields ...zap.Field) {
	l().Debug(msg, fields...)
}

// Info 记录信息级别日志
func Info(msg string, fields ...zap.Field) {
	l().Info(msg, fields...)
}

// Warn 记录警告级别日志
func Warn(msg string, fields ...zap.Field) {
	l().Warn(msg, fields...)
}

// Error 记录错误级别日志
func Error(msg string, fields ...zap.Field) {
	l().Error(msg, fields...)
}

// DPanic 记录调试panic级别日志（仅在开发模式下panic）
func DPanic(msg string, fields ...zap.Field) {
	l().DPanic(msg, fields...)
}

// Panic 记录panic级别日志（会触发panic）
func Panic(msg string, fields ...zap.Field) {
	l().Panic(msg, fields...)
}

// Fatal 记录致命级别日志（会调用os.Exit(1)）
func Fatal(msg string, fields ...zap.Field) {
	l().Fatal(msg, fields...)
}

// Sync 刷新日志缓冲区
func Sync() error {
	if log == nil {
		return zap.L().Sync()
	}
	return log.Sync()
}
