package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"go.uber.org/zap"

	"pipetGo/internal/logger"
)

var dbpool *sqlitex.Pool

// InitDB 初始化 SQLite 数据库
func InitDB(dataDir string) error {
	logger.Info("========== 开始初始化 SQLite 数据库 ==========")
	logger.Info("数据目录参数值", zap.String("dataDir", dataDir))
	logger.Info("数据目录参数是否为空", zap.Bool("isEmpty", dataDir == ""))

	// 如果配置为空，使用默认值
	if dataDir == "" {
		logger.Info("数据目录为空，使用默认值 ./sql")
		dataDir = "./sql"
	}

	logger.Info("正在尝试创建数据目录", zap.String("dataDir", dataDir))
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.Error("创建数据目录失败", zap.String("dataDir", dataDir), zap.Error(err))
		return err
	}

	// 验证目录是否真的创建成功
	if _, err := os.Stat(dataDir); err != nil {
		logger.Error("验证数据目录失败", zap.String("dataDir", dataDir), zap.Error(err))
		return err
	}
	logger.Info("数据目录创建成功并验证通过", zap.String("dataDir", dataDir))

	dbPath := filepath.Join(dataDir, "test_stats.db")
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		logger.Error("获取数据库绝对路径失败", zap.String("path", dbPath), zap.Error(err))
		return err
	}
	logger.Info("数据库路径", zap.String("path", absPath))

	dbURI := "file:" + filepath.ToSlash(absPath)
	dbpool, err = sqlitex.Open(dbURI, 0, 10)
	if err != nil {
		logger.Error("打开数据库失败", zap.String("path", absPath), zap.Error(err))
		return err
	}
	logger.Info("数据库连接打开成功", zap.String("path", absPath))

	conn := dbpool.Get(context.Background())
	defer dbpool.Put(conn)

	// 创建测试执行时间记录表
	createTableSQL := `CREATE TABLE IF NOT EXISTS test_execution_times (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		success BOOLEAN NOT NULL,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	logger.Info("正在创建表（如果不存在）")
	if err := execSQL(conn, createTableSQL); err != nil {
		logger.Error("创建表失败", zap.Error(err))
		return err
	}

	createIndexSQL := `CREATE INDEX IF NOT EXISTS idx_test_execution_times_url ON test_execution_times(url);`
	if err := execSQL(conn, createIndexSQL); err != nil {
		logger.Error("创建索引失败", zap.Error(err))
		return err
	}

	logger.Info("表创建成功或已存在")
	logger.Info("SQLite 数据库初始化成功", zap.String("path", absPath))
	return nil
}

func execSQL(conn *sqlite.Conn, query string) error {
	stmt, err := conn.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Finalize()
	_, err = stmt.Step()
	return err
}

// RecordExecutionTime 记录测试执行时间
func RecordExecutionTime(url string, duration time.Duration, success bool) error {
	if dbpool == nil {
		return fmt.Errorf("database not initialized")
	}

	conn := dbpool.Get(context.Background())
	defer dbpool.Put(conn)

	stmt := conn.Prep("INSERT INTO test_execution_times (url, duration_ms, success, executed_at) VALUES (?, ?, ?, ?)")
	stmt.BindText(1, url)
	stmt.BindInt64(2, int64(duration/time.Millisecond))
	stmt.BindBool(3, success)
	stmt.BindText(4, time.Now().Format("2006-01-02 15:04:05"))

	if _, err := stmt.Step(); err != nil {
		logger.Error("Failed to record execution time", zap.Error(err))
		return err
	}

	return nil
}

// GetAverageDuration 获取指定 URL 的平均执行时间
func GetAverageDuration(url string) (time.Duration, error) {
	if dbpool == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	conn := dbpool.Get(context.Background())
	defer dbpool.Put(conn)

	stmt := conn.Prep("SELECT AVG(duration_ms) FROM test_execution_times WHERE url = ? AND success = 1")
	stmt.BindText(1, url)

	var avgMs float64
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return 0, err
		}
		if !hasRow {
			break
		}
		avgMs = stmt.ColumnFloat(0)
	}

	return time.Duration(avgMs) * time.Millisecond, nil
}

// GetAllAverageDurations 获取所有 URL 的平均执行时间
func GetAllAverageDurations() (map[string]time.Duration, error) {
	if dbpool == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	conn := dbpool.Get(context.Background())
	defer dbpool.Put(conn)

	stmt := conn.Prep("SELECT url, AVG(duration_ms) as avg_ms FROM test_execution_times WHERE success = 1 GROUP BY url")
	averages := make(map[string]time.Duration)
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return nil, err
		}
		if !hasRow {
			break
		}
		url := stmt.ColumnText(0)
		avgMs := stmt.ColumnFloat(1)
		averages[url] = time.Duration(avgMs) * time.Millisecond
	}

	return averages, nil
}

// GetExecutionCount 获取指定 URL 的成功执行次数
func GetExecutionCount(url string) (int, error) {
	if dbpool == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	conn := dbpool.Get(context.Background())
	defer dbpool.Put(conn)

	stmt := conn.Prep("SELECT COUNT(*) FROM test_execution_times WHERE url = ? AND success = 1")
	stmt.BindText(1, url)

	var count int
	for {
		hasRow, err := stmt.Step()
		if err != nil {
			return 0, err
		}
		if !hasRow {
			break
		}
		count = stmt.ColumnInt(0)
	}

	return count, nil
}
