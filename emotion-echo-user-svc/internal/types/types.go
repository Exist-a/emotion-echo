
package types

type GetMeReq struct {
}

type GetMeResp struct {
	User UserInfo `json:"user"`
}

type GetUserByIdReq struct {
	Id int64 `path:"id"`
}

type GetUserByIdResp struct {
	User UserInfo `json:"user"`
}

type HealthResp struct {
	Status  string `json:"status"`
	Time    int64  `json:"time"`
	Service string `json:"service"`
	Version string `json:"version"`
	DbOK    bool   `json:"dbOk"`
}

type UserInfo struct {
	UserId    int64          `json:"userId"`
	Account   string         `json:"account"`
	Nickname  string         `json:"nickname"`
	AvatarURL string         `json:"avatarUrl,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt int64          `json:"createdAt,omitempty"`
}

// UpdateProfileReq PATCH /api/v1/users/me 的请求体
//
// 所有字段 optional：nil 表示不动；非 nil 表示覆盖/设置
type UpdateProfileReq struct {
	Nickname  *string        `json:"nickname,optional"`
	Gender    *int16         `json:"gender,optional"`
	Birthday  *string        `json:"birthday,optional"` // YYYY-MM-DD
	AvatarURL *string        `json:"avatarUrl,optional"`
	Config    *map[string]any `json:"config,optional"`
}

// UpdateProfileResp 与 GetMeResp 结构完全一致（用户最新信息）
type UpdateProfileResp = GetMeResp

// LoginReq POST /api/v1/users/login
//
// Stage 33 PR-19a：user-svc 真实登录。前端直接传明文密码（不再 sha256），
// user-svc 用 bcrypt.Verify 与 password_hash 比对。
type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResp 登录成功响应：返回用户信息（不含密码哈希）
type LoginResp struct {
	User UserInfo `json:"user"`
}

// SecurityQuestion 密保问题（E2E-06，供 D-01=C 找回密码）
type SecurityQuestion struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// SecurityQuestionInfo 返回给前端的密保问题信息（不含答案）
type SecurityQuestionInfo struct {
	QuestionOrder int16  `json:"questionOrder"`
	Question      string `json:"question"`
}

// GetSecurityQuestionsReq GET /api/v1/users/security-questions
type GetSecurityQuestionsReq struct {
	Username string `form:"username"`
}

// GetSecurityQuestionsResp 获取密保问题响应
type GetSecurityQuestionsResp struct {
	Questions []SecurityQuestionInfo `json:"questions"`
}

// RegisterReq POST /api/v1/users/register
//
// Stage 33 PR-19a：user-svc 注册。username 唯一；password 明文入库前
// 由 handler 调 password.Hash bcrypt 哈希。
// E2E-06: 新增 SecurityQuestions 必填（1~2 个密保问题）
type RegisterReq struct {
	Username           string             `json:"username"`
	Password           string             `json:"password"`
	VerificationCode   string             `json:"verificationCode,optional"`
	Nickname           *string            `json:"nickname,optional"`
	SecurityQuestions  []SecurityQuestion `json:"securityQuestions"`
}

// RegisterResp 注册成功响应
type RegisterResp struct {
	User UserInfo `json:"user"`
}

// Sprint 1 PR-4c-3: 重置密码请求（BFF 调）
type ResetPasswordReq struct {
	Username         string `json:"username"`
	VerificationCode string `json:"verificationCode"`
	NewPassword      string `json:"newPassword"`
}

// ResetPasswordResp 重置密码成功响应
type ResetPasswordResp struct {
	User UserInfo `json:"user"`
}

// AuthErrorResp 鉴权失败响应（统一格式）
type AuthErrorResp struct {
	Error string `json:"error"`
}

// ===================== CL-06 角色档案（用户自定义 Agent·多角色）=====================

// AgentProfileView 角色档案视图（HTTP/gRPC 共用）
type AgentProfileView struct {
	ID         int64    `json:"id"`
	UserID     int64    `json:"userId"`
	Name       string   `json:"name"`
	Gender     string   `json:"gender"`
	Persona    string   `json:"persona"`
	Tone       string   `json:"tone"`
	Boundaries []string `json:"boundaries"`
	Voice      string   `json:"voice"`
	IsDefault  bool     `json:"isDefault"`
	TemplateID string   `json:"templateId"`
	CreatedAt  int64    `json:"createdAt"`
	UpdatedAt  int64    `json:"updatedAt"`
}

// ListAgentProfilesReq 列角色请求（鉴权走 ctx，无参数）
type ListAgentProfilesReq struct{}

// ListAgentProfilesResp 列角色响应
type ListAgentProfilesResp struct {
	Items []AgentProfileView `json:"items"`
	Total int                `json:"total"`
}

// UpsertAgentProfileReq 新建 / 更新角色请求（ID=0 为新建）
type UpsertAgentProfileReq struct {
	ID         int64
	Name       string
	Gender     string
	Persona    string
	Tone       string
	Boundaries []string
	Voice      string
	IsDefault  bool
	TemplateID string
}

// DeleteAgentProfileReq 删除角色请求
type DeleteAgentProfileReq struct {
	ID int64 `path:"id"`
}

// DeleteAgentProfileResp 删除角色响应
type DeleteAgentProfileResp struct {
	Deleted bool `json:"deleted"`
}
