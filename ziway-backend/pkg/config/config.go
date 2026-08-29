package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load 加载配置，支持 prod / sqlite / dev 三种模式
func Load() (*viper.Viper, error) {
	v := viper.New()
	env := os.Getenv("APP_ENV")
	if env == "" {
		// Default to dev (SQLite) for preview/sandbox environments
		// Production should explicitly set APP_ENV=prod
		env = "dev"
	}

	switch env {
	case "sqlite", "dev":
		v.SetConfigName("dev-sqlite")
	default:
		v.SetConfigName("config")
	}

	v.SetConfigType("yaml")
	v.AddConfigPath("./configs/")
	v.AddConfigPath(".")

	// 环境变量覆盖
	v.SetEnvPrefix("ZIWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return v, nil
}
