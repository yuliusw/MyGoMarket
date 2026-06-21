package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig 全局配置实例
var AppConfig *Config

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	MinIO    MinIOConfig    `mapstructure:"minio"`
	RocketMQ RocketMQConfig `mapstructure:"rocketmq"` // 新增 MQ 配置映射
	Casbin   CasbinConfig   `mapstructure:"casbin"`
	GRPC     GRPCConfig     `mapstructure:"grpc"`
	Features FeatureConfig  `mapstructure:"features"`
}

type ServerConfig struct {
	Port                   int `mapstructure:"port"`
	ShutdownTimeoutSeconds int `mapstructure:"shutdown_timeout_seconds"`
}

type DatabaseConfig struct {
	Host                   string `mapstructure:"host"`
	Port                   int    `mapstructure:"port"`
	User                   string `mapstructure:"user"`
	Password               string `mapstructure:"password"`
	DBName                 string `mapstructure:"dbname"`
	MaxIdleConns           int    `mapstructure:"max_idle_conns"`
	MaxOpenConns           int    `mapstructure:"max_open_conns"`
	ConnMaxLifetimeSeconds int    `mapstructure:"conn_max_lifetime_seconds"`
	ConnMaxIdleTimeSeconds int    `mapstructure:"conn_max_idle_time_seconds"`
	LogLevel               string `mapstructure:"log_level"`
}

type RedisConfig struct {
	Host                string `mapstructure:"host"`
	Port                int    `mapstructure:"port"`
	Password            string `mapstructure:"password"`
	DB                  int    `mapstructure:"db"`
	PoolSize            int    `mapstructure:"pool_size"`
	MinIdleConns        int    `mapstructure:"min_idle_conns"`
	DialTimeoutSeconds  int    `mapstructure:"dial_timeout_seconds"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
	PoolTimeoutSeconds  int    `mapstructure:"pool_timeout_seconds"`
}

type MinIOConfig struct {
	Endpoint   string `mapstructure:"endpoint"`
	AccessKey  string `mapstructure:"accesskey"`
	SecretKey  string `mapstructure:"secretkey"`
	BucketName string `mapstructure:"bucketName"`
	UseSSL     bool   `mapstructure:"useSSL"`
}

// RocketMQConfig 新增 RocketMQ 配置结构体
type RocketMQConfig struct {
	// NameServer 地址列表，RocketMQ 客户端主要连接这个
	Addrs []string `mapstructure:"addrs"`
}

type CasbinConfig struct {
	ModelPath       string `mapstructure:"model_path"`
	CacheTTLSeconds int    `mapstructure:"cache_ttl_seconds"`
}

type GRPCConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`
}

type FeatureConfig struct {
	JWTAuth          bool              `mapstructure:"jwt_auth"`
	AuthBypassUserID string            `mapstructure:"auth_bypass_user_id"`
	CasbinAuthz      bool              `mapstructure:"casbin_authz"`
	CORS             bool              `mapstructure:"cors"`
	RateLimit        RateLimitConfig   `mapstructure:"rate_limit"`
	RequestPool      RequestPoolConfig `mapstructure:"request_pool"`
}

type RequestPoolConfig struct {
	Enabled  bool `mapstructure:"enabled"`
	Capacity int  `mapstructure:"capacity"`
}

type RateLimitConfig struct {
	Enabled        bool    `mapstructure:"enabled"`
	Backend        string  `mapstructure:"backend"`
	Rate           float64 `mapstructure:"rate"`
	Capacity       float64 `mapstructure:"capacity"`
	CleanupSeconds int     `mapstructure:"cleanup_seconds"`
	TTLSeconds     int     `mapstructure:"ttl_seconds"`
	WindowSeconds  int     `mapstructure:"window_seconds"`
	Limit          int     `mapstructure:"limit"`
}

// InitConfig 初始化 Viper 并读取配置文件
func InitConfig(configPath string) {
	setDefaults()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	bindEnvKeys()

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file, %s", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		log.Fatalf("Unable to decode into struct, %v", err)
	}

	log.Println("Configuration loaded successfully")
}

func bindEnvKeys() {
	keys := []string{
		"server.port",
		"server.shutdown_timeout_seconds",
		"database.host",
		"database.port",
		"database.user",
		"database.password",
		"database.dbname",
		"database.max_idle_conns",
		"database.max_open_conns",
		"database.conn_max_lifetime_seconds",
		"database.conn_max_idle_time_seconds",
		"database.log_level",
		"redis.host",
		"redis.port",
		"redis.password",
		"redis.db",
		"redis.pool_size",
		"redis.min_idle_conns",
		"redis.dial_timeout_seconds",
		"redis.read_timeout_seconds",
		"redis.write_timeout_seconds",
		"redis.pool_timeout_seconds",
		"minio.endpoint",
		"minio.accesskey",
		"minio.secretkey",
		"minio.bucketName",
		"minio.useSSL",
		"casbin.model_path",
		"casbin.cache_ttl_seconds",
		"grpc.enabled",
		"grpc.port",
		"features.jwt_auth",
		"features.auth_bypass_user_id",
		"features.casbin_authz",
		"features.cors",
		"features.rate_limit.enabled",
		"features.rate_limit.backend",
		"features.rate_limit.rate",
		"features.rate_limit.capacity",
		"features.rate_limit.cleanup_seconds",
		"features.rate_limit.ttl_seconds",
		"features.rate_limit.window_seconds",
		"features.rate_limit.limit",
		"features.request_pool.enabled",
		"features.request_pool.capacity",
	}

	for _, key := range keys {
		if err := viper.BindEnv(key); err != nil {
			log.Fatalf("Unable to bind env %s: %v", key, err)
		}
	}
}

func setDefaults() {
	viper.SetDefault("server.port", 12660)
	viper.SetDefault("server.shutdown_timeout_seconds", 15)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.max_open_conns", 100)
	viper.SetDefault("database.conn_max_lifetime_seconds", 3600)
	viper.SetDefault("database.conn_max_idle_time_seconds", 600)
	viper.SetDefault("database.log_level", "warn")
	viper.SetDefault("redis.pool_size", 100)
	viper.SetDefault("redis.min_idle_conns", 10)
	viper.SetDefault("redis.dial_timeout_seconds", 5)
	viper.SetDefault("redis.read_timeout_seconds", 3)
	viper.SetDefault("redis.write_timeout_seconds", 3)
	viper.SetDefault("redis.pool_timeout_seconds", 4)
	viper.SetDefault("casbin.model_path", "config/casbin/RBAC.conf")
	viper.SetDefault("casbin.cache_ttl_seconds", 600)
	viper.SetDefault("grpc.enabled", true)
	viper.SetDefault("grpc.port", 12661)
	viper.SetDefault("features.jwt_auth", true)
	viper.SetDefault("features.auth_bypass_user_id", "00000000-0000-0000-0000-000000000001")
	viper.SetDefault("features.casbin_authz", true)
	viper.SetDefault("features.cors", true)
	viper.SetDefault("features.rate_limit.enabled", true)
	viper.SetDefault("features.rate_limit.backend", "memory")
	viper.SetDefault("features.rate_limit.rate", 5.0)
	viper.SetDefault("features.rate_limit.capacity", 10.0)
	viper.SetDefault("features.rate_limit.cleanup_seconds", 300)
	viper.SetDefault("features.rate_limit.ttl_seconds", 600)
	viper.SetDefault("features.rate_limit.window_seconds", 1)
	viper.SetDefault("features.rate_limit.limit", 100)
	viper.SetDefault("features.request_pool.enabled", true)
	viper.SetDefault("features.request_pool.capacity", 1000)
}
