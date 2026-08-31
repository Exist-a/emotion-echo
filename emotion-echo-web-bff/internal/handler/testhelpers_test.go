// Package handler — testhelpers_test.go
//
// 测试公共 helper：解码 {code, message, data} 响应包装。
package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// decodeData 解 BFF 统一响应包装 {code, message, data}，把 data 解码到 target。
func decodeData(t *testing.T, body []byte, target any) {
	t.Helper()
	var wrapped struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &wrapped))
	require.Equal(t, 0, wrapped.Code, "code 应为 0（成功）")
	require.NoError(t, json.Unmarshal(wrapped.Data, target))
}
