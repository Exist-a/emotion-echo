package config

import (
	"strings"
	"testing"
)

// PR-OBS-3 RED: ExpandShellEnvDefaults 把 yaml/JSON 字符串中的 ${VAR} 与 ${VAR:-default}
// 替换为 os.Getenv("VAR")（空时用 default）。支持递归展开。
//
// 行为与 bash "${VAR:-default}" 一致：
//   - ${VAR}            → os.Getenv("VAR")（空时保留字面 "${VAR}" 作为未定义信号）
//   - ${VAR:-default}   → os.Getenv("VAR") ?? default（default 允许空格/嵌套占位符不可）
//   - $${VAR}           → 字面 "${VAR}"（转义，与 bash 一致）
//
// 未定义且无 default：返回 error（fail-fast）。

func TestExpandShellEnvDefaults_VarDefined(t *testing.T) {
	t.Setenv("EXPAND_TEST_HOME", "/root")
	got, err := ExpandShellEnvDefaults("${EXPAND_TEST_HOME}/x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/root/x" {
		t.Errorf("got %q, want %q", got, "/root/x")
	}
}

func TestExpandShellEnvDefaults_VarWithDefault_Defined(t *testing.T) {
	t.Setenv("EXPAND_TEST_HOME", "/root")
	got, err := ExpandShellEnvDefaults("${EXPAND_TEST_HOME:-/tmp}/x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/root/x" {
		t.Errorf("got %q, want %q", got, "/root/x")
	}
}

func TestExpandShellEnvDefaults_VarWithDefault_UndefUsesDefault(t *testing.T) {
	// EXPAND_TEST_UNSET 在测试环境保证未定义（t.Setenv("") 把它清空）
	t.Setenv("EXPAND_TEST_UNSET", "")
	got, err := ExpandShellEnvDefaults("${EXPAND_TEST_UNSET:-/tmp}/x")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/tmp/x" {
		t.Errorf("got %q, want %q", got, "/tmp/x")
	}
}

func TestExpandShellEnvDefaults_VarUndefNoDefault_ReturnsErr(t *testing.T) {
	t.Setenv("EXPAND_TEST_UNSET2", "")
	_, err := ExpandShellEnvDefaults("${EXPAND_TEST_UNSET2}/x")
	if err == nil {
		t.Fatal("expected error for undef var without default, got nil")
	}
	// 错误消息应含 var 名,便于排查
	if !strings.Contains(err.Error(), "EXPAND_TEST_UNSET2") {
		t.Errorf("error should mention var name, got: %v", err)
	}
}

func TestExpandShellEnvDefaults_DoubleDollarEscapes(t *testing.T) {
	t.Setenv("EXPAND_TEST_HOME", "/root")
	got, err := ExpandShellEnvDefaults("$${EXPAND_TEST_HOME}")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "${EXPAND_TEST_HOME}" {
		t.Errorf("got %q, want literal ${EXPAND_TEST_HOME}", got)
	}
}

func TestExpandShellEnvDefaults_MultiWordDefault(t *testing.T) {
	got, err := ExpandShellEnvDefaults("${EXPAND_TEST_UNSET3:-hello world}/y")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "hello world/y" {
		t.Errorf("got %q, want %q", got, "hello world/y")
	}
}

func TestExpandShellEnvDefaults_MultipleVarsInOneString(t *testing.T) {
	t.Setenv("EXPAND_TEST_A", "1")
	t.Setenv("EXPAND_TEST_B", "2")
	got, err := ExpandShellEnvDefaults("${EXPAND_TEST_A}-${EXPAND_TEST_B}")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "1-2" {
		t.Errorf("got %q, want %q", got, "1-2")
	}
}

func TestExpandShellEnvDefaults_UnrelatedDollarLeftAlone(t *testing.T) {
	// 只有 ${...} 触发替换;裸 $ 不替换
	got, err := ExpandShellEnvDefaults("price=$10")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "price=$10" {
		t.Errorf("got %q, want %q (unrelated $ should not trigger expansion)", got, "price=$10")
	}
}
