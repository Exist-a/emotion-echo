// Package fusion — Stage 35 fix smoke 验证
package fusion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsNilFuser_NilInterface 全 nil interface → true。
func TestIsNilFuser_NilInterface(t *testing.T) {
	t.Parallel()
	var f Fuser // nil interface
	assert.True(t, isNilFuser(f))
}

// TestIsNilFuser_NilPointerInInterface type=*LLMFuser, value=nil → true（最危险状态）。
func TestIsNilFuser_NilPointerInInterface(t *testing.T) {
	t.Parallel()
	var nilLLM *LLMFuser
	var f Fuser = nilLLM // type 已固定为 *LLMFuser，value 为 nil
	assert.True(t, isNilFuser(f), "Fuser with nil pointer must be detected")
}

// TestIsNilFuser_RealInstance 非 nil 实例 → false。
func TestIsNilFuser_RealInstance(t *testing.T) {
	t.Parallel()
	real := NewLLMFuser(LLMConfig{BaseURL: "http://x"})
	var f Fuser = real
	assert.False(t, isNilFuser(f))
}

// TestIsNilFuser_LateFuserLateFuser nil 检查同样作用于 LateFuser。
func TestIsNilFuser_LateFuser(t *testing.T) {
	t.Parallel()
	var nilLate *WeightedLateFuser
	var f Fuser = nilLate
	assert.True(t, isNilFuser(f))
}