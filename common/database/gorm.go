package database

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/yuliusw/RPA-market/common/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitGORM 初始化 PostgreSQL 数据库连接
func InitGORM() {
	dbConf := config.AppConfig.Database

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable TimeZone=Asia/Shanghai",
		dbConf.Host, dbConf.User, dbConf.Password, dbConf.DBName, dbConf.Port)

	// 配置 GORM
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel(dbConf.LogLevel)),
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	// 获取底层的 sql.DB 对象以配置连接池
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Failed to get sql.DB: %v", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(intOrDefault(dbConf.MaxIdleConns, 10))
	sqlDB.SetMaxOpenConns(intOrDefault(dbConf.MaxOpenConns, 100))
	sqlDB.SetConnMaxLifetime(durationSecondsOrDefault(dbConf.ConnMaxLifetimeSeconds, time.Hour))
	sqlDB.SetConnMaxIdleTime(durationSecondsOrDefault(dbConf.ConnMaxIdleTimeSeconds, 10*time.Minute))

	log.Println("PostgreSQL connected successfully")
}

func gormLogLevel(level string) logger.LogLevel {
	switch strings.ToLower(level) {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "info":
		return logger.Info
	default:
		return logger.Warn
	}
}

func intOrDefault(value, defaultValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return value
}

func durationSecondsOrDefault(seconds int, defaultValue time.Duration) time.Duration {
	if seconds <= 0 {
		return defaultValue
	}
	return time.Duration(seconds) * time.Second
}

func CloseGORM() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
