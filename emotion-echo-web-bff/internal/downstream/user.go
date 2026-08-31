// Package downstream — user.go
//
// Stage 30 / stage-30-web-bff.md T2.12-14: UserClient（BFF → user-svc）
// Stage 32 PR-16: 鉴权改用 X-User-Id header。
//
// user-svc（Gin :8888）：
//   GET   /api/v1/users/me     → {"user": UserInfo}
//   PATCH /api/v1/users/me     → {"user": UserInfo}（UpdateProfileReq 全 optional 指针）
//   GET   /api/v1/users/:id    → {"user": UserInfo}
//
// 鉴权：下游认 X-User-Id header（APISIX jwt-auth 注入）。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// UserInfo 对应 user-svc types.UserInfo
type UserInfo struct {
	UserID   int64  `json:"userId"`
	Account  string `json:"account"`
	Phone    string `json:"phone"`
	Nickname string `json:"nickname"`
}

// UpdateProfileReq 对应 user-svc types.UpdateProfileReq（全 optional）
type UpdateProfileReq struct {
	Nickname  *string `json:"nickname,omitempty"`
	Gender    *int16  `json:"gender,omitempty"`
	Birthday  *string `json:"birthday,omitempty"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

// userWrapper 响应外层统一包 {"user": UserInfo}
type userWrapper struct {
	User *UserInfo `json:"user"`
}

// UserClient BFF → user-svc HTTP 客户端
type UserClient interface {
	// GetMe 获取当前用户（JWT 识别调用者）
	GetMe(ctx context.Context) (*UserInfo, error)
	// UpdateMe 更新当前用户资料
	UpdateMe(ctx context.Context, req UpdateProfileReq) (*UserInfo, error)
	// GetByID 按 ID 查用户
	GetByID(ctx context.Context, id int64) (*UserInfo, error)

	// Stage 33 PR-19b: 真实登录（POST /api/v1/users/login）
	// 不走 ctx X-User-Id（因为 login 时调用者未认证）
	// 返回 user-svc 校验后的 UserInfo；user-svc 返 401 时回 ErrInvalidCredentials
	Login(ctx context.Context, username, password string) (*UserInfo, error)
	// Stage 33 PR-19b: 真实注册（POST /api/v1/users/register）
	Register(ctx context.Context, username, password, verificationCode string) (*UserInfo, error)
}

// UserClientOptions 构造选项
type UserClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// userHTTPClient 是 UserClient 的 HTTP 实现
type userHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewUserClient 构造 UserClient
func NewUserClient(opts UserClientOptions) UserClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &userHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *userHTTPClient) GetMe(ctx context.Context) (*UserInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/users/me", nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(httpReq, ctx)
	return c.doUserRequest(httpReq)
}

func (c *userHTTPClient) UpdateMe(ctx context.Context, req UpdateProfileReq) (*UserInfo, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal update profile: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/api/v1/users/me", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq, ctx)
	return c.doUserRequest(httpReq)
}

func (c *userHTTPClient) GetByID(ctx context.Context, id int64) (*UserInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/users/"+strconv.FormatInt(id, 10), nil)
	if err != nil {
		return nil, err
	}
	c.applyAuth(httpReq, ctx)
	return c.doUserRequest(httpReq)
}

// Stage 33 PR-19b: login（不走 ctx X-User-Id）
func (c *userHTTPClient) Login(ctx context.Context, username, password string) (*UserInfo, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/users/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// 不注入 X-User-Id（未认证）
	return c.doUserRequest(httpReq)
}

// Stage 33 PR-19b: register（不走 ctx X-User-Id）
func (c *userHTTPClient) Register(ctx context.Context, username, password, verificationCode string) (*UserInfo, error) {
	body, _ := json.Marshal(map[string]string{
		"username":         username,
		"password":         password,
		"verificationCode": verificationCode,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/users/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return c.doUserRequest(httpReq)
}

func (c *userHTTPClient) applyAuth(req *http.Request, ctx context.Context) {
	if uid, ok := UserIDFromContext(ctx); ok && uid > 0 {
		req.Header.Set(XUserIDHeader, strconv.FormatInt(uid, 10))
	}
}

func (c *userHTTPClient) doUserRequest(req *http.Request) (*UserInfo, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: user request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var wrapped userWrapper
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("downstream: decode user resp: %w", err)
	}
	if wrapped.User == nil {
		return nil, fmt.Errorf("downstream: user response missing 'user' field")
	}
	return wrapped.User, nil
}
