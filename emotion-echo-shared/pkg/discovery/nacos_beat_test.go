// Package discovery — Round 4.2 P1-7 BeatInstance 测试
package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendBeatInstance_OK 正常路径：服务端返回 clientBeatInterval 解析正确
func TestSendBeatInstance_OK(t *testing.T) {
	var gotPath, gotMethod string
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		gotForm = make(map[string]string)
		for k, v := range r.PostForm {
			if len(v) > 0 {
				gotForm[k] = v[0]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"clientBeatInterval": 5000})
	}))
	defer srv.Close()

	interval, err := sendBeatInstance(
		context.Background(),
		srv.URL, // 完整 URL（"http://127.0.0.1:NNNN"）
		"default",
		"DEFAULT_GROUP",
		"web-bff",
		BeatInfo{
			ClusterName: "DEFAULT",
			IP:          "127.0.0.1",
			Port:        8894,
			Weight:      1.0,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, interval, "解析 clientBeatInterval=5000 → 5s")

	assert.Equal(t, "/nacos/v1/ns/instance/beat", gotPath, "必须 POST /instance/beat 标准路径")
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "web-bff", gotForm["serviceName"])
	assert.Equal(t, "DEFAULT_GROUP", gotForm["groupName"])
	assert.Equal(t, "127.0.0.1", gotForm["ip"])
	assert.Equal(t, "8894", gotForm["port"])
	assert.NotEmpty(t, gotForm["beat"], "必须带 beat JSON")
}

// TestSendBeatInstance_ServerError 5xx 时返 error
func TestSendBeatInstance_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", 500)
	}))
	defer srv.Close()

	_, err := sendBeatInstance(context.Background(), srv.URL, "default", "DEFAULT_GROUP", "web-bff", BeatInfo{
		ClusterName: "DEFAULT", IP: "127.0.0.1", Port: 8894, Weight: 1.0,
	})
	assert.Error(t, err, "服务端 5xx 必须返 error（让 caller 退化为 UpdateInstance）")
	assert.Contains(t, err.Error(), "500")
}

// TestSendBeatInstance_EmptyServerAddr 空地址返 error
func TestSendBeatInstance_EmptyServerAddr(t *testing.T) {
	_, err := sendBeatInstance(context.Background(), "", "default", "DEFAULT_GROUP", "web-bff", BeatInfo{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty serverAddr")
}

// TestBeatInfo_Marshal 验证 BeatInfo JSON 字段名与 Java 客户端一致
func TestBeatInfo_Marshal(t *testing.T) {
	beat := BeatInfo{ClusterName: "DEFAULT", IP: "1.2.3.4", Port: 80, Weight: 1.0}
	b, err := json.Marshal(beat)
	require.NoError(t, err)
	// Java 客户端使用 clusterName / ip / port / weight 字段名（无 clientBeatInterval）
	assert.Contains(t, string(b), `"clusterName":"DEFAULT"`)
	assert.Contains(t, string(b), `"ip":"1.2.3.4"`)
	assert.Contains(t, string(b), `"port":80`)
	assert.Contains(t, string(b), `"weight":1`)
}
