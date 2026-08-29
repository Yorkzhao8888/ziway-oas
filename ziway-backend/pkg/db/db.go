package db

import (
	"context"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 初始化数据库（支持 postgres / sqlite）
func InitDB(v *viper.Viper, log *zap.Logger) (*gorm.DB, error) {
	driver := v.GetString("database.driver")
	if driver == "" {
		driver = "postgres"
	}

	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		path := v.GetString("database.sqlite_path")
		if path == "" {
			path = "ziway.db"
		}
		dialector = sqlite.Open(path)
		log.Info("using SQLite", zap.String("path", path))
	default:
		dsn := v.GetString("database.dsn")
		if dsn == "" {
			return nil, fmt.Errorf("database.dsn is required for postgres driver")
		}
		dialector = postgres.Open(dsn)
		log.Info("using PostgreSQL")
	}

	gormCfg := &gorm.Config{}
	if v.GetString("app.env") != "prod" {
		gormCfg.Logger = logger.Default.LogMode(logger.Info)
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(v.GetInt("database.max_open_conns"))
	sqlDB.SetMaxIdleConns(v.GetInt("database.max_idle_conns"))
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Info("database connected", zap.String("driver", driver))
	return db, nil
}

// AutoMigrate 自动建表
func AutoMigrate(db *gorm.DB, models ...interface{}) error {
	return db.AutoMigrate(models...)
}

// InitRedis 初始化 Redis 客户端（支持降级模式）
func InitRedis(v *viper.Viper, log *zap.Logger) *redis.Client {
	if v.GetBool("dev_no_redis") {
		log.Warn("Redis disabled (dev_no_redis=true)")
		return redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     v.GetString("redis.addr"),
		Password: v.GetString("redis.password"),
		DB:       v.GetInt("redis.db"),
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Warn("Redis ping failed, running in degraded mode", zap.Error(err))
	} else {
		log.Info("Redis connected")
	}
	return rdb
}
