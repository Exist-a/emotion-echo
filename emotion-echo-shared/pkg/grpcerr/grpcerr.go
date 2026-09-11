// Package grpcerr 提供 Emotion-Echo 后端 5 个 svc 共享的"业务错误 → gRPC code" 映射。
//
// 设计动机（2026-09-11 Sprint 决策 4 收口 ADR §八 backlog）：
//
//	4 svc 各有自己的 mapLogicError / mapAuthError / mapAIError / status.Errorf 散写，
//	共 19 处重复 + 4 套并行逻辑。本包统一收口：
//	  1. 通用 sentinel errors（ErrInvalidArgument / ErrUnauthenticated / ...）
//	  2. 业务 sentinel 注册机制（MapError）
//	  3. errors.Is 优先匹配
//	  4. 字符串前缀兜底（兼容历史契约如 "unauthorized: missing user id"）
//	  5. 默认 fallback codes.Internal
//
// 用法（chat-svc / user-svc / ai-svc / analytics-svc / assessment-svc gRPC server）：
//
//	return nil, grpcerr.Wrap(err, "list conversations")
//
// 或自定义 code：
//
//	return nil, grpcerr.MapToError(err, "delete conversation")
package grpcerr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 通用 sentinel errors — 各 svc 直接 errors.Is 或 wrap 后返回。
//
// 这些 err 共享 errors.Is 判定，所以业务代码可以这样写：
//
//	return grpcerr.ErrNotFound
// 或
//
//	return fmt.Errorf("user lookup: %w", grpcerr.ErrNotFound)
var (
	ErrInvalidArgument  = errors.New("invalid argument")
	ErrUnauthenticated  = errors.New("unauthenticated")
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotFound         = errors.New("not found")
	ErrUnavailable      = errors.New("unavailable")
	ErrAlreadyExists    = errors.New("already exists")
)

// customMap 业务 sentinel → gRPC code 的注册表。
// 用 sync.RWMutex  保护并发读写（main.go 初始化期 vs grpc server 并发调 Map）。
var (
	customMapMu sync.RWMutex
	customMap   = map[error]codes.Code{}
)

// MapError 注册业务 sentinel error → gRPC code 的映射。
//
// 注册后，Map(err) 会优先 errors.Is 该 err 返回对应 code。
// 同一 err 重复注册后者覆盖前者。线程安全。
//
// 典型用例（user-svc grpcserver 包 init()）：
//
//	grpcerr.MapError(ErrInvalidCredentials, codes.Unauthenticated)
//	grpcerr.MapError(ErrUsernameTaken, codes.AlreadyExists)
func MapError(businessErr error, code codes.Code) {
	customMapMu.Lock()
	defer customMapMu.Unlock()
	customMap[businessErr] = code
}

// 字符串前缀规则（兜底匹配，按最长前缀优先）。
// 顺序敏感：先列的规则先匹配。
var stringPrefixes = []struct {
	prefix string
	code   codes.Code
}{
	{"unauthorized", codes.Unauthenticated},
	{"forbidden", codes.PermissionDenied},
	{"validation", codes.InvalidArgument},
	{"not found", codes.NotFound},
	{"conflict", codes.AlreadyExists},
	{"unavailable", codes.Unavailable},
	// ai-svc：XTTS / SenseVoice / FER 模型容器不可用时错误信息前缀（cf. aiclient/xtts.go:98 "call XTTS: ...""）
	{"call xtts", codes.Unavailable},
	{"xtts", codes.Unavailable},
	{"sensevoice", codes.Unavailable},
	{"fer", codes.Unavailable},
	{"llm service", codes.Unavailable},
}

// Map 把任意 error 映射到 (gRPC code, message)。
//
// 匹配顺序：
//  1. err == nil → OK, ""
//  2. err 已是 gRPC status.Error → 返回原 code + message（防止双重 wrap）
//  3. errors.Is 命中 MapError 注册的 sentinel → 返回注册 code
//  4. errors.Is 命中 6 类通用 sentinel → 返回对应 code
//  5. err.Error() 命中字符串前缀规则 → 返回对应 code
//  6. errors.Is(err, context.DeadlineExceeded) → DeadlineExceeded
//  7. 默认 → codes.Internal + err.Error()
//
// 返回 message 是给客户端的 err.Error()，原始 err 仍可通过 errors.As 还原（调用方需要）。
func Map(err error) (codes.Code, string) {
	if err == nil {
		return codes.OK, ""
	}

	// 包装的 gRPC status error：直接返回原 code（防止双重 wrap 改语义）
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return st.Code(), st.Message()
	}

	// 业务 sentinel errors（用户注册）
	customMapMu.RLock()
	for sentinelErr, code := range customMap {
		if errors.Is(err, sentinelErr) {
			customMapMu.RUnlock()
			return code, err.Error()
		}
	}
	customMapMu.RUnlock()

	// 通用 sentinel errors
	switch {
	case errors.Is(err, ErrInvalidArgument):
		return codes.InvalidArgument, err.Error()
	case errors.Is(err, ErrUnauthenticated):
		return codes.Unauthenticated, err.Error()
	case errors.Is(err, ErrPermissionDenied):
		return codes.PermissionDenied, err.Error()
	case errors.Is(err, ErrNotFound):
		return codes.NotFound, err.Error()
	case errors.Is(err, ErrUnavailable):
		return codes.Unavailable, err.Error()
	case errors.Is(err, ErrAlreadyExists):
		return codes.AlreadyExists, err.Error()
	}

	// 字符串前缀兜底
	msg := err.Error()
	lowMsg := strings.ToLower(msg)
	for _, p := range stringPrefixes {
		if strings.HasPrefix(lowMsg, p.prefix) {
			return p.code, msg
		}
	}

	// context deadline（gRPC 默认 DeadlineExceeded 与 HTTP 504 对齐）
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded, msg
	}

	// 默认 fallback
	return codes.Internal, msg
}

// MapToError 便利方法：err → gRPC status.Error。
//
// 等价于：
//
//	code, msg := grpcerr.Map(err)
//	return status.Error(code, msg)
//
// 但 err 透传给调用方（status.Error 会把 err 链截断；调用方如需 errors.Is 原始 err，
// 应自行 wrap 而不依赖 MapToError）。
func MapToError(err error, op string) error {
	code, msg := Map(err)
	if op != "" {
		msg = op + ": " + msg
	}
	return status.Error(code, msg)
}

// Wrap 把任意 err 转成 status.Error，并在 message 前加 op 标识。
//
// 推荐用法：
//
//	return nil, grpcerr.Wrap(err, "list conversations")
//
// 行为：
//   - err == nil → 返回 nil（不构造空 status）
//   - err 已是 status.Error → 直接返回原 err（避免双重 wrap）
//   - 其他 → 调 Map(err) 映射 + op 前缀构造 status.Error
func Wrap(err error, op string) error {
	if err == nil {
		return nil
	}
	// 已 wrap 过的不再 wrap（防止 message 嵌套）
	if _, ok := status.FromError(err); ok {
		return err
	}
	code, msg := Map(err)
	fullMsg := msg
	if op != "" {
		fullMsg = op + ": " + msg
	}
	return status.Error(code, fullMsg)
}

// ResetForTest 清理 MapError 注册表（仅供测试用）。
//
// 不导出到生产代码路径，避免误用破坏业务 sentinel 注册。
func ResetForTest() {
	customMapMu.Lock()
	defer customMapMu.Unlock()
	customMap = map[error]codes.Code{}
}

// 确保 fmt 在某些 build tag 下也被引用（防止 goimports 误删）
var _ = fmt.Sprintf