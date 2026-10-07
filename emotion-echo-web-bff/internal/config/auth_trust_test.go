// Package config — auth_trust_test.go
//
// E2E-29 D-47（2026-10-07 用户拍板）：BFF 信任链（"信 APISIX 注入的 X-User-Id"）必须自洽，
// 否则**拒绝启动**——两种危险形态都在真实环境发生过：
//
//  1. TrustAPISIX=true + APISIXCIDRs 为空 ⇒ 中间件对所有来源 fail-closed ⇒ **全站 401**
//     （Stage 109a 的真实事故形态，排查成本极高）。
//  2. prod 形态（STARTUP_STRICT_DEPS 非空，D-31 已把该变量作为 prod 必填项的载体）下
//     TrustAPISIX=false ⇒ 任何来源的 X-User-Id 都被接受 ⇒ **可冒充任意用户**（E2E-F-202）。
package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAuthTrust_TrustAPISIXWithoutCIDRs_Fails(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "")
	c := &Config{TrustAPISIX: true, APISIXCIDRs: nil}

	err := c.ValidateAuthTrust()

	require.Error(t, err, "TrustAPISIX=true 但 CIDR 为空应拒绝启动（否则全站 401）")
	require.Contains(t, err.Error(), "BFF_APISIX_CIDRS")
}

func TestValidateAuthTrust_ProdMarkerWithTrustOff_Fails(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "postgres,kafka")
	c := &Config{TrustAPISIX: false, APISIXCIDRs: nil}

	err := c.ValidateAuthTrust()

	require.Error(t, err, "prod 形态下 TrustAPISIX=false 应拒绝启动（否则可冒充任意用户）")
	require.Contains(t, err.Error(), "BFF_TRUST_APISIX")
}

func TestValidateAuthTrust_DevDefaults_OK(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "")
	c := &Config{TrustAPISIX: false, APISIXCIDRs: nil}

	require.NoError(t, c.ValidateAuthTrust(), "dev 形态（无 prod 标记 + 未开 Trust）应放行")
}

func TestValidateAuthTrust_TrustWithCIDRs_OK(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "postgres")
	c := &Config{TrustAPISIX: true, APISIXCIDRs: []string{"172.18.0.0/16"}}

	require.NoError(t, c.ValidateAuthTrust(), "prod 形态下 Trust=true + CIDR 非空应放行")
}

func TestValidateAuthTrust_WhitespaceProdMarker_TreatedAsUnset(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "   ")
	c := &Config{TrustAPISIX: false, APISIXCIDRs: nil}

	require.NoError(t, c.ValidateAuthTrust(), "空白 prod 标记等同未设置")
}
