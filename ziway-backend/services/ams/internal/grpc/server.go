package grpc

import (
	"context"
	"net"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Server gRPC服务器（供其他微服务调用）
type Server struct {
	server       *grpc.Server
	port         int
	serviceToken string
	logger       *zap.Logger
}

// Config gRPC配置
type Config struct {
	Port         int
	ServiceToken string
	Logger       *zap.Logger
}

// NewServer 创建gRPC服务器
func NewServer(cfg *Config) *Server {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(serviceTokenInterceptor(cfg.ServiceToken, cfg.Logger)),
	}

	return &Server{
		server:       grpc.NewServer(opts...),
		port:         cfg.Port,
		serviceToken: cfg.ServiceToken,
		logger:       cfg.Logger,
	}
}

// Start 启动gRPC服务
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", ":"+itoa(s.port))
	if err != nil {
		return err
	}

	s.logger.Info("gRPC server starting", zap.Int("port", s.port))
	go func() {
		if err := s.server.Serve(lis); err != nil {
			s.logger.Fatal("gRPC server failed", zap.Error(err))
		}
	}()
	return nil
}

// Stop 优雅关闭
func (s *Server) Stop() {
	s.server.GracefulStop()
}

// GRPCServer 返回底层grpc.Server用于注册service
func (s *Server) GRPCServer() *grpc.Server {
	return s.server
}

// serviceTokenInterceptor 服务间Token验证拦截器
func serviceTokenInterceptor(expectedToken string, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Health check放行
		if info.FullMethod == "/grpc.health.v1.Health/Check" {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		tokens := md.Get("x-service-token")
		if len(tokens) == 0 {
			logger.Warn("gRPC call missing service token",
				zap.String("method", info.FullMethod),
			)
			return nil, status.Error(codes.Unauthenticated, "missing service token")
		}

		if tokens[0] != expectedToken {
			logger.Warn("gRPC call invalid service token",
				zap.String("method", info.FullMethod),
			)
			return nil, status.Error(codes.PermissionDenied, "invalid service token")
		}

		return handler(ctx, req)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
