package logs

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.Logger

// InitLogger 初始化日志
func InitLogger() {
	encoderConfig := zap.NewProductionEncoderConfig()
	// 调整时间格式，方便 Loki 索引
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig), // PLG 推荐使用 JSON
		zapcore.AddSync(os.Stdout),            // 标准输出由 Promtail 收集
		zap.NewAtomicLevelAt(zap.InfoLevel),
	)

	Log = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

// WithContext 让日志带上业务链路标识
func WithContext(ctx context.Context) *zap.SugaredLogger {
	if Log == nil {
		return zap.NewNop().Sugar()
	}
	logger := Log.Sugar()
	// 尝试从 ctx 中获取 trace_id (由中间件注入)
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		return logger.With("trace_id", traceID)
	}
	return logger
}
