// Package fusion — nil 防御 helper
//
// isNilFuser 检测 Fuser interface 是不是"类型有 + value nil"的危险状态。
//
// 背景：Go interface 的零值是 (nil type, nil value)；一旦给它赋值，type 就固定了，
// 但 value 可以仍是 nil。简单的 `f == nil` 只查两者都 nil 的情况，不会暴露
// "type=*LLMFuser, value=nil" 的危险状态——调用方法会 panic。
//
// Stage 35 PR-5 后的副作用：main.go 里 `var llmFuser *fusion.LLMFuser` 当
// LLM_BASE_URL 为空时保持 nil，被赋给 `Fuser` interface 后 type 已固定但 value nil。
// worker 之前的 `if w.deps.LLMFuser != nil` 守卫对此无效 → panic on Fuse。
package fusion

import (
	"reflect"
)

// isNilFuser 返回 true 当 f 是 nil 或者 type+nil value 状态。
//
// 使用 reflect.ValueOf().IsNil()，对 pointer / channel / func / map / interface 都安全。
func isNilFuser(f Fuser) bool {
	if f == nil {
		return true
	}
	v := reflect.ValueOf(f)
	switch v.Kind() {
	case reflect.Ptr, reflect.Chan, reflect.Func, reflect.Map, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}