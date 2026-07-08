package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/sqlite"
	"go.uber.org/zap"

	"pipetGo/internal/logger"
)

var db *sql.DB

// InitDB 初始化 SQLite 数据库
func InitDB(dataDir string) error {
	// 确保数据目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	dbPath := filepath.Join(dataDir, "test_stats.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	// 创建测试执行时间记录表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS test_execution_times (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		success BOOLEAN NOT NULL,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_test_execution_times_url ON test_execution_times(url);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return err
	}

	logger.Info("SQLite database initialized", zap.String("path", dbPath))
	return nil
}

// RecordExecutionTime 记录测试执行时间
func RecordExecutionTime(url string, duration time.Duration, success bool) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(
		"INSERT INTO test_execution_times (url, duration_ms, success, executed_at) VALUES (?, ?, ?, ?)",
		url,
		int64(duration/time.Millisecond),
		success,
		time.Now(),
	)

	if err != nil {
		logger.Error("Failed to record execution time", zap.Error(err))
		return err
	}

	return nil
}

// GetAverageDuration 获取指定 URL 的平均执行时间
func GetAverageDuration(url string) (time.Duration, error) {
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var avgMs float64
	err := db.QueryRow(
		"SELECT AVG(duration_ms) FROM test_execution_times WHERE url = ? AND success = 1",
		url,
	).Scan(&avgMs)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // 没有历史记录
		}
		return 0, err
	}

	return time.Duration(avgMs) * time.Millisecond, nil
}

// GetAllAverageDurations 获取所有 URL 的平均执行时间
func GetAllAverageDurations() (map[string]time.Duration, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`
		SELECT url, AVG(duration_ms) as avg_ms
		FROM test_execution_times
		WHERE success = 1
		GROUP BY url
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	averages := make(map[string]time.Duration)
	for rows.Next() {
		var url string
		var avgMs float64
		if err := rows.Scan(&url, &avgMs); err != nil {
			return nil, err
		}
		averages[url] = time.Duration(avgMs) * time.Millisecond
	}

	return averages, nil
}

// GetExecutionCount 获取指定 URL 的成功执行次数
func GetExecutionCount(url string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM test_execution_times WHERE url = ? AND success = 1",
		url,
	).Scan(&count)

	if err != nil {
		return 0, err
	}

	return count, nil
}