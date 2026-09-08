package pkg

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 初始化数据库连接
// 支持 PostgreSQL（生产）和 SQLite（开发/演示）
// 配置 database.driver = "postgres"（默认）或 "sqlite"
func InitDB(v *viper.Viper, log *zap.Logger) (*gorm.DB, error) {
	driver := v.GetString("database.driver")
	if driver == "" {
		driver = "postgres"
	}

	var dialector gorm.Dialector
	switch driver {
	case "sqlite":
		dbPath := v.GetString("database.sqlite_path")
		if dbPath == "" {
			dbPath = "ziway_ams.db"
		}
		dialector = sqlite.Open(dbPath)
		log.Info("using SQLite", zap.String("path", dbPath))
	default:
		dsn := fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			v.GetString("database.host"),
			v.GetInt("database.port"),
			v.GetString("database.user"),
			v.GetString("database.password"),
			v.GetString("database.dbname"),
			v.GetString("database.sslmode"),
		)
		dialector = postgres.Open(dsn)
	}

	var logLevel logger.LogLevel
	switch v.GetString("database.log_level") {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	default:
		logLevel = logger.Info
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if driver == "postgres" {
		sqlDB.SetMaxIdleConns(v.GetInt("database.max_idle_conns"))
		sqlDB.SetMaxOpenConns(v.GetInt("database.max_open_conns"))
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	log.Info("database connected",
		zap.String("driver", driver),
	)
	return db, nil
}

// InitRedis 初始化Redis连接
// 支持 Redis 和 内存模式（dev_no_redis = true 时跳过）
func InitRedis(v *viper.Viper, log *zap.Logger) (*redis.Client, error) {
	// 开发模式：不连接Redis，使用空客户端
	if v.GetBool("dev_no_redis") {
		log.Info("Redis disabled (dev_no_redis=true)")
		return redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
			DB:   0,
		}), nil
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", v.GetString("redis.host"), v.GetInt("redis.port")),
		Password: v.GetString("redis.password"),
		DB:       v.GetInt("redis.db"),
		PoolSize: v.GetInt("redis.pool_size"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Warn("Redis connection failed, running in degraded mode", zap.Error(err))
		// 降级模式：返回客户端但不强制连接
		return rdb, nil
	}

	log.Info("redis connected",
		zap.String("host", v.GetString("redis.host")),
	)
	return rdb, nil
}
