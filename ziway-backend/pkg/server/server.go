package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Config 服务器配置
type Config struct {
	ServiceName string
	HTTPPort    string
	GRPCPort    string
	Logger      *zap.Logger
}

// Start 同时启动HTTP和gRPC服务器，支持优雅关闭
func Start(cfg Config, httpHandler *gin.Engine, grpcServer *grpc.Server) {
	// HTTP
	httpAddr := ":" + cfg.HTTPPort
	if cfg.HTTPPort == "" {
		httpAddr = ":8080"
	}
	httpSrv := &http.Server{
		Addr:    httpAddr,
		Handler: httpHandler,
	}

	// gRPC (optional)
	var grpcLn net.Listener
	if cfg.GRPCPort != "" && grpcServer != nil {
		var err error
		grpcLn, err = net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			cfg.Logger.Fatal("gRPC listen failed", zap.Error(err))
		}
		go func() {
			cfg.Logger.Info("gRPC server started", zap.String("addr", ":"+cfg.GRPCPort))
			if err := grpcServer.Serve(grpcLn); err != nil {
				cfg.Logger.Error("gRPC serve error", zap.Error(err))
			}
		}()
	}

	// HTTP
	go func() {
		cfg.Logger.Info(fmt.Sprintf("🚀 %s HTTP server started on %s", cfg.ServiceName, httpAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			cfg.Logger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cfg.Logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if grpcServer != nil {
		grpcServer.GracefulStop()
	}
	httpSrv.Shutdown(ctx)
	cfg.Logger.Info("server stopped gracefully")
}
