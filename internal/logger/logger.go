package logger

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.Logger

type LogConfig struct {
	Level    string
	Encoding string
	Output   string
}

func InitLogger(cfg LogConfig) {
	var zapConfig zap.Config

	switch cfg.Encoding {
	case "console":
		zapConfig = zap.NewDevelopmentConfig()
	default:
		zapConfig = zap.NewProductionConfig()
	}

	zapConfig.Level = zap.NewAtomicLevelAt(getLogLevel(cfg.Level))
	zapConfig.Encoding = cfg.Encoding

	var outputPaths []string
	if cfg.Output == "stdout" {
		outputPaths = []string{"stdout"}
	} else {
		logPath := addTimestampToFilename(cfg.Output)
		ensureDir(logPath)
		outputPaths = []string{logPath}
	}
	zapConfig.OutputPaths = outputPaths

	loc, _ := time.LoadLocation("Asia/Shanghai")
	zapConfig.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.In(loc).Format("2006-01-02 15:04:05.000"))
	}

	var err error
	log, err = zapConfig.Build()
	if err != nil {
		zap.L().Fatal("Failed to initialize logger", zap.Error(err))
		os.Exit(1)
	}

	zap.ReplaceGlobals(log)
}

func addTimestampToFilename(path string) string {
	dir := filepath.Dir(path)
	filename := filepath.Base(path)
	
	ext := filepath.Ext(filename)
	nameWithoutExt := strings.TrimSuffix(filename, ext)
	
	timestamp := time.Now().Format("20060102_150405")
	
	return filepath.Join(dir, nameWithoutExt+"_"+timestamp+ext)
}

func ensureDir(path string) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		os.MkdirAll(dir, 0755)
	}
}


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

func Debug(msg string, fields ...zap.Field) {
	log.Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	log.Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	log.Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	log.Error(msg, fields...)
}

func DPanic(msg string, fields ...zap.Field) {
	log.DPanic(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	log.Panic(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	log.Fatal(msg, fields...)
}

func Sync() error {
	return log.Sync()
}