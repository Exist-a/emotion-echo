// Package config 提供 emotion-echo-web-bff 的配置结构（Stage 41 PR-7）
//
// Stage 41 改造点:去掉所有 json:",default=X" tag + json:",optional" tag,
// 改为 SetDefaults 函数(shared/pkg/config 契约);yaml 字段大小写映射沿用
// shared/pkg/config 的 lowercaseYAMLKeys 预处理(go-zero 不展开 ${VAR:-default},
// 占位符字面值原样保留)。
package config

import (
	"fmt"
	"os"
	"strings"
)

// IsProdMarked 判定"生产形态"。
//
// 复用既有变量而非新造开关：STARTUP_STRICT_DEPS 是仓库既有且**只在 prod 非空**的
// fail-fast 载体（shared/pkg/bootstrap/deps.go；D-31 亦以它为 prod 必填项的口径，
// compose.prod.yml TODO 1b 同源）。空白串等同未设置。
func IsProdMarked() bool {
	return strings.TrimSpace(os.Getenv("STARTUP_STRICT_DEPS")) != ""
}

// ValidateAuthTrust 校验"信任 APISIX 注入的 X-User-Id"这一安全前提是否自洽（E2E-29 D-47）。
//
// 两种危险形态都在真实环境发生过，且**都表现为线上异常而非启动失败**：
//  1. TrustAPISIX=true + APISIXCIDRs 为空 ⇒ 中间件对**所有**来源 fail-closed
//     （`gin_auth.go` 的 `cidrs == nil` 分支）⇒ 全站 401（Stage 109a 事故形态）。
//  2. prod 形态（STARTUP_STRICT_DEPS 非空）下 TrustAPISIX=false ⇒ 任何来源的
//     X-User-Id 均被接受 ⇒ 可冒充任意用户（E2E-F-202：直连 BFF 端口伪造 header）。
//
// 让它们在**启动时**失败，而不是在流量上失败。
func (c *Config) ValidateAuthTrust() error {
	if c.TrustAPISIX && len(c.APISIXCIDRs) == 0 {
		return fmt.Errorf("BFF_TRUST_APISIX=true 但 BFF_APISIX_CIDRS 为空：中间件将拒绝所有来源" +
			"（全站 401，Stage 109a 事故形态）；请设可信 APISIX 网段，或显式关掉 BFF_TRUST_APISIX")
	}
	if !c.TrustAPISIX && IsProdMarked() {
		return fmt.Errorf("prod 形态（STARTUP_STRICT_DEPS 非空）下 BFF_TRUST_APISIX 必须为 true：" +
			"false 会接受任意来源的 X-User-Id（可冒充任意用户，见账本 E2E-F-202）")
	}
	return nil
}

// SkyWalking 链路追踪配置（与 chat-svc 同构）
type SkyWalking struct {
	OAPAddr     string
	ServiceName string
	Enabled     bool
}

// HTTPService 是下游 HTTP 服务的通用配置
//
// Stage 63: 新增 GRPCAddr + Transport 字段，让 BFF 能 dial 下游 gRPC server。
// GRPCAddr 为空 → 不 dial，下游工厂静默走 HTTP fallback。
// Transport 为空 → 下游工厂按 "grpc" 处理（需同时 GRPCConn 非 nil）；
// Transport="http" → 强制 HTTP（回滚开关）。
type HTTPService struct {
	BaseURL   string
	TimeoutMs int
	GRPCAddr  string
	Transport string
}

// AIService 是 ai-svc 的双协议配置（HTTP + gRPC）
type AIService struct {
	HTTPAddr  string
	GRPCAddr  string
	TimeoutMs int
	// Sprint F2（2026-09-11）：加 Transport 字段，与 4 svc 同模式。
	// 默认空 → 工厂按 "grpc" 处理；= "http" → 强制 HTTP 回滚。
	Transport string
}

// Config 是 BFF 总配置
//
// 字段名与 etc/web-bff.yaml 一一对应。
type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Nacos      Nacos

	UserService       HTTPService
	ChatService       HTTPService
	AssessmentService HTTPService
	AnalyticsService  HTTPService

	AIService AIService

	XTTS HTTPService

	Health struct {
		TimeoutMs int
	}

	// Auth 是 BFF 自己的 JWT 配置（Stage 33 PR-19b 真实登录）
	Auth struct {
		JWTSecret        string
		TokenTTLSeconds  int
	}

	// TrustAPISIX 控制 BFF 是否信任 APISIX 注入的 X-User-Id header
	TrustAPISIX bool
	// APISIXCIDRs Stage 94 PR-6 §P0-7：可信 APISIX IP 段(CIDR),用于
	// GinAuthMiddlewareWithOpts 的 RequireAPISIXIP 校验。svc 端口直连场景
	// 攻击者伪造 X-User-Id 必须来自这些 IP 才被信任。dev 模式留空 +
	// TrustAPISIX=false 时跳过 IP 校验。
	APISIXCIDRs []string

	// LLM 是 BFF ai_stream 调用的真实 LLM（OpenAI 兼容）
	LLM struct {
		BaseURL string
		APIKey  string
		Model   string
		Timeout int
		// Stage 81 PR-2：llm-service ChatCompletion gRPC 上游地址（LLM_SVC_GRPC_ADDR；
		// 非空 = 启用 gRPC 优先路径，llm-service 未注册 Nacos 故仅 env 直连）
		GRPCAddr string
	}

	// Sprint 1 PR-4b: MinIO 对象存储
	MinIO struct {
		Endpoint      string
		AccessKey     string
		SecretKey     string
		Bucket        string
		UseSSL        bool
		PublicBaseURL string
	}

	// TTS 是语音合成上游配置（E2E-F-198，ADR-2026-10 / 架构决策 40）
	//
	// Provider 语义（downstream.NewTTSProvider 语义矩阵）：
	//   auto（默认）+ APIKey 空 → 直接 XTTS（dev 常态，无降级日志）
	//   auto + APIKey 非空     → cloud（CosyVoice2 API）优先，失败降级 XTTS + Warn
	//   cloud                  → 只用 cloud，失败即失败（fail-loud）
	//   local                  → 只用 XTTS（回滚开关）
	//
	// ⚠️ APIKey 恒不入 yaml（同 LLM 段纪律）：只进 gitignored .env.local（TTS_API_KEY）。
	TTS struct {
		Provider       string // auto | cloud | local
		BaseURL        string // cloud API base（含 /v1）
		APIKey         string
		Model          string
		Voice          string // 预置音色带模型名前缀（官方约定）
		SampleRate     int    // 必须 24000（与 XTTS SAMPLE_RATE 对齐；官方默认 44100 不可用）
		ResponseFormat string // wav（前端 base64ToWavBlob 不构造 WAV 头）
		Timeout        int    // 秒
	}
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
type Nacos struct {
	Enabled   bool
	Addr      string
	Namespace string
	GroupName string
	HotReload bool
}

// SetDefaults 填零值字段的默认值（shared/pkg/config.MustLoad 契约）。
//
// R3 反向:Kafka.Enabled / SkyWalking.Enabled / Nacos.Enabled / Nacos.HotReload /
// TrustAPISIX / MinIO.UseSSL 等 bool 不在 SetDefaults 里覆盖（yaml 显式 false
// 必须保留）。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-web-bff"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8894
	}
	if c.SkyWalking.OAPAddr == "" {
		c.SkyWalking.OAPAddr = "localhost:11800"
	}
	// 下游服务默认
	setHTTPServiceDefaults(&c.UserService, "http://localhost:8888")
	setHTTPServiceDefaults(&c.ChatService, "http://localhost:8890")
	setHTTPServiceDefaults(&c.AssessmentService, "http://localhost:8889")
	setHTTPServiceDefaults(&c.AnalyticsService, "http://localhost:8904")
	// Stage 63: 下游 gRPC 地址默认值（与各 svc grpcserver 监听端口对齐）
	if c.UserService.GRPCAddr == "" {
		c.UserService.GRPCAddr = "localhost:8887"
	}
	if c.ChatService.GRPCAddr == "" {
		c.ChatService.GRPCAddr = "localhost:8892"
	}
	if c.AssessmentService.GRPCAddr == "" {
		c.AssessmentService.GRPCAddr = "localhost:8886"
	}
	if c.AnalyticsService.GRPCAddr == "" {
		c.AnalyticsService.GRPCAddr = "localhost:8885"
	}
	if c.AIService.HTTPAddr == "" {
		c.AIService.HTTPAddr = "http://localhost:8891"
	}
	if c.AIService.GRPCAddr == "" {
		c.AIService.GRPCAddr = "localhost:8892"
	}
	if c.AIService.TimeoutMs == 0 {
		// E2E-F-115（2026-09-22）：原默认 5000ms（5s），dev 模式下首次 SenseVoice 转写
		// （冷启动：ffmpeg 解码 + 首次张量分配）≈5~15s，5s 必撞 504 DeadlineExceeded。
		// 改 30000ms（30s）—— 容纳冷启动，又给 image/short 留充足余量。
		c.AIService.TimeoutMs = 30000
	}
	setHTTPServiceDefaults(&c.XTTS, "http://localhost:8003")
	if c.XTTS.TimeoutMs == 0 {
		// E2E-F-138（2026-09-23 IAB 用户实测「嘴动没声音」判别实验）：
		// XTTS CPU 推理 49 字（AI 正常回复长度）实测 103.6s ≈ 2.1s/字 + 15s 开销；
		// 原 90000ms 只够 ~35 字 → AI 回复必撞 502 → 前端无音频 → 永远没声音。
		// 180000ms（180s）覆盖 ~80 字；同步覆盖 /tts/stream 路径（同一 NewXTTSClient）。
		// yaml 与本默认必须一致（yaml 非 0 时此处不覆盖 → 漂移即事故；
		// 测试 TestConfig_XTTSDefaultTimeoutIs180s 钉守卫）。
		// 真实可听性根因 = F-132（XTTS 2 核限额）：本字段单独不够，8 核下 49 字仅 19.9s。
		c.XTTS.TimeoutMs = 180000
	}
	if c.Health.TimeoutMs == 0 {
		c.Health.TimeoutMs = 2000
	}
	if c.Auth.JWTSecret == "" {
		// Stage 94 PR-5 §P0-10：删 dev-bff-secret 默认值。漏 env 注入时
		// 保持空 → main.go 调 auth.NewManager(secret, ttl) 时 NewManager
		// 内部 fail-fast ("auth: JWT secret must not be empty") + log.Fatal。
		// 这是 §P0-10 修复目标:避免 dev 密钥进生产(所有 JWT 用 dev 密钥签 →
		// 攻击者可伪造任意 user_id token)。
	}
	if c.Auth.TokenTTLSeconds == 0 {
		c.Auth.TokenTTLSeconds = 86400
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "https://api.deepseek.com"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "deepseek-chat"
	}
	if c.LLM.Timeout == 0 {
		c.LLM.Timeout = 60
	}
	// E2E-F-198（ADR-2026-10）：默认值与 etc/web-bff.yaml TTS 段必须一致
	// （yaml 非 0 非空时此处不覆盖；漂移即事故——TestConfig_TTSDefaults 钉守卫）。
	if c.TTS.Provider == "" {
		c.TTS.Provider = "auto"
	}
	if c.TTS.BaseURL == "" {
		c.TTS.BaseURL = "https://api.siliconflow.cn/v1"
	}
	if c.TTS.Model == "" {
		c.TTS.Model = "FunAudioLLM/CosyVoice2-0.5B"
	}
	if c.TTS.Voice == "" {
		// 预置音色带模型名前缀（官方约定）；换 TTS_MODEL 时需同步 TTS_VOICE 前缀
		c.TTS.Voice = "FunAudioLLM/CosyVoice2-0.5B:anna"
	}
	if c.TTS.SampleRate == 0 {
		// 官方默认 44100 不可用：XTTS SAMPLE_RATE=24000，前端 duration 算术与
		// 音调都依赖 24k（plan §B.4）
		c.TTS.SampleRate = 24000
	}
	if c.TTS.ResponseFormat == "" {
		// 前端 base64ToWavBlob 不构造 WAV 头 ⇒ 必须 wav 容器（plan §B.3）
		c.TTS.ResponseFormat = "wav"
	}
	if c.TTS.Timeout == 0 {
		// cloud 实测 33 字完整返回 ~1.5s（ADR §实证），30s 极宽裕
		c.TTS.Timeout = 30
	}
	if c.MinIO.Endpoint == "" {
		c.MinIO.Endpoint = "emotion-echo-minio:9000"
	}
	if c.MinIO.AccessKey == "" {
		c.MinIO.AccessKey = "minioadmin"
	}
	if c.MinIO.SecretKey == "" {
		c.MinIO.SecretKey = "minioadmin"
	}
	if c.MinIO.Bucket == "" {
		c.MinIO.Bucket = "avatars"
	}
	if c.MinIO.PublicBaseURL == "" {
		c.MinIO.PublicBaseURL = "http://localhost:9000"
	}
	if c.Nacos.Addr == "" {
		c.Nacos.Addr = "emotion-echo-nacos:8848"
	}
	if c.Nacos.Namespace == "" {
		c.Nacos.Namespace = "emotion-echo-dev"
	}
	if c.Nacos.GroupName == "" {
		c.Nacos.GroupName = "DEFAULT_GROUP"
	}
}

func setHTTPServiceDefaults(s *HTTPService, defaultBaseURL string) {
	if s.BaseURL == "" {
		s.BaseURL = defaultBaseURL
	}
	if s.TimeoutMs == 0 {
		s.TimeoutMs = 5000
	}
	// Transport 默认空 → 下游工厂按 "grpc" 处理（与决策 4 "内部 svc-to-svc = gRPC" 对齐）。
	//
	// Sprint C（2026-09-11）历史：曾因 grpcinterceptor.CtxUserIDKeyType 与 middleware.CtxUserIDKey
	// 两个类型不一致（决策 18 #26）临时默认 "http" 绕路。Sprint C ctxkey 重构后两类型别名指向
	// ctxkey.UserID，跨包 ctx 读写一致，可恢复 grpc 默认。
	//
	// Sprint D（chat-svc 6 RPC 实现）未做前，chat-svc gRPC 仍返 Unimplemented；
	// 届时 BFF 端 conversations 仍 500，需要 Sprint D 解决。
}

// ApplyEnvOverrides 用容器环境变量覆盖 config 字段（Stage 22-B 范式）。
//
// 覆盖项与 etc/web-bff.yaml 注释的 env 名一一对应：
//   USER_SVC_URL / CHAT_SVC_URL / ASSESSMENT_SVC_URL / ANALYTICS_SVC_URL
//   AI_SVC_HTTP_URL / AI_SVC_GRPC_ADDR / XTTS_BASE_URL / SKYWALKING_OAP_ADDR
//
// 仅覆盖非空 env（空串视为未设置，保持 yaml 默认值）。
func ApplyEnvOverrides(c *Config) {
	if v := os.Getenv("USER_SVC_URL"); v != "" {
		c.UserService.BaseURL = v
	}
	if v := os.Getenv("CHAT_SVC_URL"); v != "" {
		c.ChatService.BaseURL = v
	}
	if v := os.Getenv("ASSESSMENT_SVC_URL"); v != "" {
		c.AssessmentService.BaseURL = v
	}
	if v := os.Getenv("ANALYTICS_SVC_URL"); v != "" {
		c.AnalyticsService.BaseURL = v
	}
	// Stage 63: 下游 gRPC 地址 env 覆盖（容器 DNS）
	if v := os.Getenv("USER_SVC_GRPC_ADDR"); v != "" {
		c.UserService.GRPCAddr = v
	}
	if v := os.Getenv("CHAT_SVC_GRPC_ADDR"); v != "" {
		c.ChatService.GRPCAddr = v
	}
	if v := os.Getenv("ASSESSMENT_SVC_GRPC_ADDR"); v != "" {
		c.AssessmentService.GRPCAddr = v
	}
	if v := os.Getenv("ANALYTICS_SVC_GRPC_ADDR"); v != "" {
		c.AnalyticsService.GRPCAddr = v
	}
	// Stage 63: 传输协议回滚开关（默认空 → grpc；设 "http" 强制走 HTTP fallback）
	if v := os.Getenv("USER_TRANSPORT"); v != "" {
		c.UserService.Transport = v
	}
	if v := os.Getenv("CHAT_TRANSPORT"); v != "" {
		c.ChatService.Transport = v
	}
	if v := os.Getenv("ASSESSMENT_TRANSPORT"); v != "" {
		c.AssessmentService.Transport = v
	}
	if v := os.Getenv("ANALYTICS_TRANSPORT"); v != "" {
		c.AnalyticsService.Transport = v
	}
	if v := os.Getenv("AI_TRANSPORT"); v != "" {
		c.AIService.Transport = v
	}
	if v := os.Getenv("AI_SVC_HTTP_URL"); v != "" {
		c.AIService.HTTPAddr = v
	}
	if v := os.Getenv("AI_SVC_GRPC_ADDR"); v != "" {
		c.AIService.GRPCAddr = v
	}
	if v := os.Getenv("XTTS_BASE_URL"); v != "" {
		c.XTTS.BaseURL = v
	}
	if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
		c.SkyWalking.OAPAddr = v
	}
	if v := os.Getenv("SKYWALKING_ENABLED"); v != "" {
		c.SkyWalking.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("BFF_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("BFF_TRUST_APISIX"); v != "" {
		c.TrustAPISIX = v == "true" || v == "1"
	}
	// Stage 94 PR-6 §P0-7:逗号分隔 APISIX CIDR 列表(默认 k8s pod CIDR)
	if v := os.Getenv("BFF_APISIX_CIDRS"); v != "" {
		var cidrs []string
		for _, c := range strings.Split(v, ",") {
			if t := strings.TrimSpace(c); t != "" {
				cidrs = append(cidrs, t)
			}
		}
		c.APISIXCIDRs = cidrs
	}
	if v := os.Getenv("BFF_LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("BFF_LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
	if v := os.Getenv("BFF_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	if v := os.Getenv("LLM_SVC_GRPC_ADDR"); v != "" {
		c.LLM.GRPCAddr = v
	}
	// E2E-F-198：TTS API 配置注入（key 红线同 BFF_LLM_API_KEY——只进 gitignored
	// .env.local，compose 用 ${TTS_API_KEY:-} 空默认，缺 key 自动降级 XTTS）
	if v := os.Getenv("TTS_PROVIDER"); v != "" {
		c.TTS.Provider = v
	}
	if v := os.Getenv("TTS_API_KEY"); v != "" {
		c.TTS.APIKey = v
	}
	if v := os.Getenv("TTS_API_BASE_URL"); v != "" {
		c.TTS.BaseURL = v
	}
	if v := os.Getenv("TTS_MODEL"); v != "" {
		c.TTS.Model = v
	}
	if v := os.Getenv("TTS_VOICE"); v != "" {
		c.TTS.Voice = v
	}
	// Stage 31 PR-09: Nacos
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		c.Nacos.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("NACOS_ADDR"); v != "" {
		c.Nacos.Addr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		c.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_HOT_RELOAD"); v != "" {
		c.Nacos.HotReload = v == "true" || v == "1"
	}
	// Sprint 1 PR-4b: MinIO
	if v := os.Getenv("MINIO_ENDPOINT"); v != "" {
		c.MinIO.Endpoint = v
	}
	if v := os.Getenv("MINIO_ACCESS_KEY"); v != "" {
		c.MinIO.AccessKey = v
	}
	if v := os.Getenv("MINIO_SECRET_KEY"); v != "" {
		c.MinIO.SecretKey = v
	}
	if v := os.Getenv("MINIO_BUCKET"); v != "" {
		c.MinIO.Bucket = v
	}
	if v := os.Getenv("MINIO_PUBLIC_BASE_URL"); v != "" {
		c.MinIO.PublicBaseURL = v
	}
}
