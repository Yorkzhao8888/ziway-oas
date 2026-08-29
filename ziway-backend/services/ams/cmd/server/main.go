package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"ziway.ams/internal/authz"
	"ziway.ams/internal/grpc"
	httpServer "ziway.ams/internal/http"
	"ziway.ams/internal/jwt"
	"ziway.ams/internal/model"
	"ziway.ams/internal/pkg"
	"ziway.ams/internal/repository"
	"ziway.ams/internal/service"
)

func main() {
	// 1. 加载配置
	v := viper.New()
	v.SetConfigName("dev")
	v.SetConfigType("yaml")
	v.AddConfigPath("configs")
	v.AddConfigPath(".")

	switch os.Getenv("APP_ENV") {
	case "prod":
		v.SetConfigName("prod")
	case "sqlite":
		v.SetConfigName("dev-sqlite")
	}
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Sprintf("read config: %v", err))
	}

	// 2. 初始化Logger
	var logger *zap.Logger
	var err error
	if v.GetString("log.format") == "json" {
		logger, err = zap.NewProduction()
	} else {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		panic(fmt.Sprintf("init logger: %v", err))
	}
	defer logger.Sync()

	logger.Info("starting ziway-ams",
		zap.String("env", v.GetString("server.mode")),
		zap.Int("http_port", v.GetInt("server.http_port")),
		zap.Int("grpc_port", v.GetInt("server.grpc_port")),
	)

	// 3. 连接数据库
	db, err := pkg.InitDB(v, logger)
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}

	// 4. 自动迁移（开发阶段，生产用migrations）
	if err := db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
	); err != nil {
		logger.Fatal("auto migrate failed", zap.Error(err))
	}
	logger.Info("database migration completed")

	// 5. 连接Redis
	rdb, err := pkg.InitRedis(v, logger)
	if err != nil {
		logger.Fatal("redis connection failed", zap.Error(err))
	}

	// 6. 初始化JWT签发器和验签器
	accessTTL := v.GetDuration("jwt.access_token_ttl")
	refreshTTL := v.GetDuration("jwt.refresh_token_ttl")

	issuer, err := jwt.NewIssuer(
		v.GetString("jwt.private_key_path"),
		accessTTL, refreshTTL,
		v.GetString("jwt.issuer"),
	)
	if err != nil {
		logger.Fatal("init jwt issuer failed", zap.Error(err))
	}

	verifier, err := jwt.NewVerifier(v.GetString("jwt.public_key_path"))
	if err != nil {
		logger.Fatal("init jwt verifier failed", zap.Error(err))
	}
	logger.Info("JWT RS256 keypair loaded")

	// 7. 初始化Casbin
	enforcer, err := authz.NewEnforcer(db, v.GetString("casbin.model_path"), logger)
	if err != nil {
		logger.Fatal("init casbin failed", zap.Error(err))
	}

	// 8. 初始化Repository
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)

	// 9. 确保默认角色存在
	if err := roleRepo.EnsureDefaultRoles(); err != nil {
		logger.Warn("ensure default roles failed", zap.Error(err))
	} else {
		logger.Info("default roles ensured")
	}

	// 10. 初始化Service
	authSvc := service.NewAuthService(
		userRepo, roleRepo, issuer, verifier, enforcer, rdb, logger,
	)

	// 11. 启动gRPC服务器
	grpcSrv := grpc.NewServer(&grpc.Config{
		Port:         v.GetInt("server.grpc_port"),
		ServiceToken: v.GetString("service.token"),
		Logger:       logger,
	})
	if err := grpcSrv.Start(); err != nil {
		logger.Fatal("grpc server failed", zap.Error(err))
	}

	// 12. 启动HTTP服务器
	router := httpServer.NewRouter(&httpServer.RouterConfig{
		AuthSvc:      authSvc,
		UserRepo:     userRepo,
		RoleRepo:     roleRepo,
		Verifier:     verifier,
		Enforcer:     enforcer,
		RDB:          rdb,
		ServiceToken: v.GetString("service.token"),
		Logger:       logger,
	})

	httpAddr := fmt.Sprintf(":%d", v.GetInt("server.http_port"))
	go func() {
		logger.Info("HTTP server starting", zap.String("addr", httpAddr))
		if err := router.Run(httpAddr); err != nil {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// 13. 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcSrv.Stop()

	select {
	case <-ctx.Done():
		logger.Info("server exited")
	}
}
