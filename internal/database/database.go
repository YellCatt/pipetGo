package database

import (
	"fmt"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"pipet/config"
	"pipet/internal/logger"
	"pipet/internal/models"
)

var DB *gorm.DB

func InitDB() {
	var db *gorm.DB
	var err error

	cfg := config.AppConfig.Database

	switch cfg.Driver {
	case "mysql":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.DBName)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.DBName)
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	case "sqlite3":
		db, err = gorm.Open(sqlite.Open(cfg.DBName), &gorm.Config{})
	default:
		logger.Fatal("Unsupported database driver", zap.String("driver", cfg.Driver))
	}

	if err != nil {
		logger.Fatal("Failed to connect database", zap.Error(err))
	}

	DB = db

	if err := autoMigrate(); err != nil {
		logger.Error("Failed to auto migrate", zap.Error(err))
	}

	logger.Info("Database connected successfully", zap.String("driver", cfg.Driver))
}

func autoMigrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.Product{},
	)
}
