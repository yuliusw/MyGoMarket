package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yuliusw/RPA-market/common/audit"
	"github.com/yuliusw/RPA-market/common/config"
	"github.com/yuliusw/RPA-market/common/database"
	"github.com/yuliusw/RPA-market/common/grpcserver"
	"github.com/yuliusw/RPA-market/common/middleware"
	"github.com/yuliusw/RPA-market/common/queue/rocketmq"
	"github.com/yuliusw/RPA-market/common/utils"
	"github.com/yuliusw/RPA-market/common/utils/logs"
	"github.com/yuliusw/RPA-market/common/utils/pool"
	"github.com/yuliusw/RPA-market/services/admin"
	"github.com/yuliusw/RPA-market/services/iam"
	"github.com/yuliusw/RPA-market/services/market"
	"github.com/yuliusw/RPA-market/services/order"
	"github.com/yuliusw/RPA-market/services/wallet"
	"google.golang.org/grpc"
)

func main() {
	logs.InitLogger()
	defer logs.Log.Sync()

	// 1. 加载配置 (注意：确保项目根目录下有 config.yaml)
	log.Println("Initializing configuration...")
	config.InitConfig("config.yaml")

	// 2. 初始化数据库和缓存
	log.Println("Initializing database and Redis...")
	database.InitGORM()
	audit.Start(database.DB)
	database.InitRedis()
	database.InitMinio()
	audit.StartMinioDeleteRetryWorker(database.DB, database.GlobalMinio)
	if err := rocketmq.Init(); err != nil {
		log.Fatalf("Failed to initialize RocketMQ: %v", err)
	}
	initAuthorizationSync()
	RegisterService()
	// 3. 初始化 Gin 引擎
	r := gin.Default()
	r.Use(middleware.GinLogger())
	r.Use(middleware.ConfiguredRequestPoolFastFail())

	// 4. 注册所有路由 (将 Gin 引擎实例传递给 IAM 服务)
	log.Println("Registering routes...")
	iam.RegisterHandlers(r)
	market.RegisterMarketHandlers(r)
	order.RegisterOrderHandlers(r)
	wallet.RegisterWalletHandlers(r)
	admin.RegisterAdminHandlers(r)
	// 5. 启动服务并监听退出信号
	port := fmt.Sprintf(":%d", intOrDefault(config.AppConfig.Server.Port, 12660))
	server := &http.Server{
		Addr:    port,
		Handler: r,
	}
	grpcServer := grpcserver.New(database.DB)

	serverErr := make(chan error, 1)
	startGRPCServer(grpcServer, serverErr)
	go func() {
		log.Printf("Server is running at http://localhost%s\n", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	case <-shutdownCtx.Done():
		gracefulShutdown(server, grpcServer)
	}
}

func initAuthorizationSync() {
	if config.AppConfig == nil || !config.AppConfig.Features.CasbinAuthz {
		log.Println("Casbin authorization disabled; skipping Casbin cache and RocketMQ sync startup")
		return
	}

	if err := rocketmq.GetClient().StartProducer("PG_casbin_sync"); err != nil {
		log.Fatalf("Failed to start RocketMQ producer: %v", err)
	}
	utils.InitCasbinPoolWithTTL(
		database.DB,
		config.AppConfig.Casbin.ModelPath,
		1000,
		time.Duration(config.AppConfig.Casbin.CacheTTLSeconds)*time.Second,
	)
	if err := rocketmq.InitCasbinSyncConsumer("Topic_Casbin_Sync", "GID_casbin_sync_group"); err != nil {
		log.Fatalf("Failed to start Casbin sync consumer: %v", err)
	}
}

func startGRPCServer(server *grpc.Server, serverErr chan<- error) {
	if config.AppConfig == nil || !config.AppConfig.GRPC.Enabled {
		log.Println("gRPC server disabled")
		return
	}

	addr := fmt.Sprintf(":%d", intOrDefault(config.AppConfig.GRPC.Port, 12661))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC address %s: %v", addr, err)
	}

	go func() {
		log.Printf("gRPC server is running at localhost%s\n", addr)
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()
}

func gracefulShutdown(server *http.Server, grpcServer *grpc.Server) {
	timeout := time.Duration(intOrDefault(config.AppConfig.Server.ShutdownTimeoutSeconds, 15)) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Shutting down server, timeout=%s", timeout)
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown failed: %v", err)
	} else {
		log.Println("HTTP server stopped")
	}
	stopGRPCServer(ctx, grpcServer)
	audit.StopMinioDeleteRetryWorker()
	audit.Shutdown(ctx)

	pool.CloseAllWithTimeout(timeout)
	if client := rocketmq.GetClient(); client != nil {
		client.Close()
	}
	if err := database.CloseRedis(); err != nil {
		log.Printf("Redis close failed: %v", err)
	}
	if err := database.CloseGORM(); err != nil {
		log.Printf("PostgreSQL close failed: %v", err)
	}
	log.Println("Graceful shutdown completed")
}

func stopGRPCServer(ctx context.Context, server *grpc.Server) {
	if server == nil || config.AppConfig == nil || !config.AppConfig.GRPC.Enabled {
		return
	}

	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Println("gRPC server stopped")
	case <-ctx.Done():
		server.Stop()
		log.Println("gRPC server force stopped")
	}
}

func intOrDefault(value, defaultValue int) int {
	if value <= 0 {
		return defaultValue
	}
	return value
}
