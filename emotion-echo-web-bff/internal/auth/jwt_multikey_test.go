// Package auth — jwt_multikey_test.go
//
// E2E-29 D-48（2026-10-07 用户拍板「双密钥并存窗口」）：JWT 密钥轮换必须**不打断在途会话**。
//
// 背景（账本 E2E-F-28）：`BFF_JWT_SECRET` 由 BFF 与 APISIX consumer 共用，单密钥下轮换
// 必然让所有在途 token 失效（全员被迫重登）。双密钥窗口的做法：
//   - 签发用**新**密钥，token 的 `key` claim 携带**新 key id**；
//   - 验签接受新旧两把（按 token 的 key claim 选密钥，缺失/未知则依次试）；
//   - 网关侧靠 key claim 找 consumer ⇒ 窗口期需为**新 key id 也建一条 consumer**
//     （见 deploy/apisix/seed.sh 的 `$JWT_KEY_V2`），旧 key id 的 consumer 保留到窗口结束。
//
// 本文件钉住 BFF 侧的双密钥语义。
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	keyIDNew  = "user-v2"
	keyIDOld  = "user"
	secretNew = "secret-new-2026"
	secretOld = "secret-old-2025"
)

func TestManagerMulti_SignUsesNewKeyID(t *testing.T) {
	m, err := NewManagerMulti(secretNew, secretOld, keyIDNew, keyIDOld, 3600)
	require.NoError(t, err)

	tokenStr, err := m.Sign(42, "alice")
	require.NoError(t, err)

	claims := &Claims{}
	_, _, err = jwt.NewParser().ParseUnverified(tokenStr, claims)
	require.NoError(t, err)
	assert.Equal(t, keyIDNew, claims.Key, "签发必须带新 key id（网关据此选 consumer）")
	assert.Equal(t, keyIDNew, claims.User, "user claim 与 key 同步（APISIX 两种 key_claim_name 都能命中）")
}

func TestManagerMulti_ParseAcceptsCurrentAndPrevious(t *testing.T) {
	m, err := NewManagerMulti(secretNew, secretOld, keyIDNew, keyIDOld, 3600)
	require.NoError(t, err)

	oldMgr, err := NewManagerMulti(secretOld, "", keyIDOld, "", 3600)
	require.NoError(t, err)
	oldToken, err := oldMgr.Sign(7, "bob")
	require.NoError(t, err)

	newToken, err := m.Sign(42, "alice")
	require.NoError(t, err)

	uid, err := m.Parse(newToken)
	require.NoError(t, err, "新密钥签的 token 必须可验")
	assert.Equal(t, int64(42), uid)

	uid, err = m.Parse(oldToken)
	require.NoError(t, err, "窗口期必须仍接受上一把密钥签的 token（否则在途会话全断）")
	assert.Equal(t, int64(7), uid)
}

func TestManagerMulti_ParseRejectsUnknownSecret(t *testing.T) {
	m, err := NewManagerMulti(secretNew, secretOld, keyIDNew, keyIDOld, 3600)
	require.NoError(t, err)

	foreign, err := NewManagerMulti("totally-different", "", keyIDNew, "", 3600)
	require.NoError(t, err)
	foreignToken, err := foreign.Sign(1, "eve")
	require.NoError(t, err)

	_, err = m.Parse(foreignToken)
	assert.Error(t, err, "无关密钥签的 token 必须拒绝（双密钥不是无限放行）")
}

func TestManagerMulti_ParseRejectsExpiredEvenWithPrevKey(t *testing.T) {
	m, err := NewManagerMulti(secretNew, secretOld, keyIDNew, keyIDOld, 3600)
	require.NoError(t, err)

	claims := Claims{
		UserID: 9, User: keyIDOld, Key: keyIDOld, Username: "carol",
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			Subject:   "9",
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secretOld))
	require.NoError(t, err)

	_, err = m.Parse(expired)
	assert.Error(t, err, "过期 token 在窗口期也必须拒绝（双密钥只放宽密钥，不放宽 exp）")
}

func TestManagerMulti_Validation(t *testing.T) {
	_, err := NewManagerMulti(secretNew, "", "", "", 3600)
	assert.Error(t, err, "key id 为空应报错")

	_, err = NewManagerMulti(secretNew, secretOld, keyIDNew, keyIDNew, 3600)
	assert.Error(t, err, "新旧 key id 相同应报错（网关无法区分两把密钥）")

	_, err = NewManagerMulti(secretNew, "", keyIDNew, keyIDOld, 3600)
	assert.Error(t, err, "给了 prev key id 却没给 prev secret 应报错")

	_, err = NewManagerMulti("", "", keyIDNew, "", 3600)
	assert.Error(t, err, "当前密钥为空应报错")
}

func TestNewManager_DefaultsToLegacyKeyID(t *testing.T) {
	m, err := NewManager(secretNew, 3600)
	require.NoError(t, err)
	tokenStr, err := m.Sign(1, "u")
	require.NoError(t, err)
	claims := &Claims{}
	_, _, err = jwt.NewParser().ParseUnverified(tokenStr, claims)
	require.NoError(t, err)
	assert.Equal(t, "user", claims.Key, "向后兼容：不配 key id 时仍用历史值 user")
}
