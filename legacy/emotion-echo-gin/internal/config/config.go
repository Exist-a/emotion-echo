package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	AI        AIConfig        `mapstructure:"ai"`
	OAuth     OAuthConfig     `mapstructure:"oauth"`
	Storage   StorageConfig   `mapstructure:"storage"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Log       LogConfig       `mapstructure:"log"`
	Analysis  AnalysisConfig  `mapstructure:"analysis"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	Mode         string `mapstructure:"mode"`
	ReadTimeout  int    `mapstructure:"read_timeout"`
	WriteTimeout int    `mapstructure:"write_timeout"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
}

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret             string `mapstructure:"secret"`
	AccessTokenExpire  int    `mapstructure:"access_token_expire"`  // 分钟
	RefreshTokenExpire int    `mapstructure:"refresh_token_expire"` // 小时
}

// AIConfig AI 配置
type AIConfig struct {
	Provider string        `mapstructure:"provider"`
	Kimi     KimiConfig    `mapstructure:"kimi"`
	Local    LocalAIConfig `mapstructure:"local"`
	ASR      ASRConfig    `mapstructure:"asr"`
	Emotion  EmotionConfig `mapstructure:"emotion"`
	Context  ContextConfig `mapstructure:"context"`
}

// ContextConfig 上下文管理配置
type ContextConfig struct {
	Type           string `mapstructure:"type"` // "token" or "window" or "buffer"
	MaxTokens      int    `mapstructure:"max_tokens"`
	WindowSize     int    `mapstructure:"window_size"`
}

// ASRConfig ASR 配置
type ASRConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

// EmotionConfig 情绪识别配置
type EmotionConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	BaseURL  string `mapstructure:"base_url"`
}

// KimiConfig Kimi 配置
type KimiConfig struct {
	APIKey      string  `mapstructure:"api_key"`
	BaseURL     string  `mapstructure:"base_url"`
	Model       string  `mapstructure:"model"`
	Timeout     int     `mapstructure:"timeout"`
	MaxTokens   int     `mapstructure:"max_tokens"`
	Temperature float64 `mapstructure:"temperature"`
}

// LocalAIConfig 本地 AI 配置
type LocalAIConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Timeout int    `mapstructure:"timeout"`
}

// OAuthConfig 第三方登录配置
type OAuthConfig struct {
	WechatAppID       string `mapstructure:"wechat_app_id"`
	WechatAppSecret   string `mapstructure:"wechat_app_secret"`
	WechatRedirectURI string `mapstructure:"wechat_redirect_uri"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type  string       `mapstructure:"type"`
	Local LocalStorage `mapstructure:"local"`
}

// LocalStorage 本地存储配置
type LocalStorage struct {
	Path    string `mapstructure:"path"`
	BaseURL string `mapstructure:"base_url"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerSecond int  `mapstructure:"requests_per_second"`
	Burst             int  `mapstructure:"burst"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// AnalysisConfig 情绪分析配置
type AnalysisConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	CronSchedule      string `mapstructure:"cron_schedule"`
	ThresholdMessages int    `mapstructure:"threshold_messages"`
	TimeoutMinutes    int    `mapstructure:"timeout_minutes"`
}

// GetPostgresDSN 获取 PostgreSQL DSN（使用 URL 格式确保 dbname 生效）
func (c *Config) GetPostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.Database.Postgres.User,
		c.Database.Postgres.Password,
		c.Database.Postgres.Host,
		c.Database.Postgres.Port,
		c.Database.Postgres.DBName,
		c.Database.Postgres.SSLMode,
	)
}

// GetRedisAddr 获取 Redis 地址
func (c *Config) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Database.Redis.Host, c.Database.Redis.Port)
}

// GetAccessTokenExpire 获取访问令牌过期时间
func (c *Config) GetAccessTokenExpire() time.Duration {
	return time.Duration(c.JWT.AccessTokenExpire) * time.Minute
}

// GetRefreshTokenExpire 获取刷新令牌过期时间
func (c *Config) GetRefreshTokenExpire() time.Duration {
	return time.Duration(c.JWT.RefreshTokenExpire) * time.Hour
}

// Load 加载配置
func Load() (*Config, error) {
	// 加载 .env 文件（如果存在）
	if err := godotenv.Load(); err != nil {
		// .env 文件不存在时不报错，使用系统环境变量
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env: %w", err)
		}
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")

	// 环境变量覆盖（前缀 EE_）
	viper.SetEnvPrefix("EE")
	viper.AutomaticEnv()
	// 修复环境变量映射：将 viper 键中的 "." 替换为 "_"，
	// 使 EE_AI_KIMI_API_KEY 能正确映射到 ai.kimi.api_key
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 默认值
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("server.read_timeout", 30)
	viper.SetDefault("server.write_timeout", 30)
	viper.SetDefault("database.postgres.host", "localhost")
	viper.SetDefault("database.postgres.port", 5432)
	viper.SetDefault("database.postgres.sslmode", "disable")
	viper.SetDefault("database.redis.host", "localhost")
	viper.SetDefault("database.redis.port", 6379)
	viper.SetDefault("jwt.access_token_expire", 15)
	viper.SetDefault("jwt.refresh_token_expire", 168)
	viper.SetDefault("rate_limit.enabled", true)
	viper.SetDefault("rate_limit.requests_per_second", 10)
	viper.SetDefault("rate_limit.burst", 20)
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("analysis.enabled", true)
	viper.SetDefault("analysis.cron_schedule", "0 3 * * *")
	viper.SetDefault("analysis.threshold_messages", 20)
	viper.SetDefault("analysis.timeout_minutes", 30)
	viper.SetDefault("ai.context.type", "token")
	viper.SetDefault("ai.context.max_tokens", 5000)
	viper.SetDefault("ai.context.window_size", 20)

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
