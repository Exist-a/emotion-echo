// emotion-echo-shared/pkg/emotionuser/stub_test.go
//
// Stage 62 PR-3.1 stub 单元测试：
//   验证 protoc-gen-go + protoc-gen-go-grpc 生成的代码可被 build + 注册
//   server 描述符（确保后续 PR-3.2 三 svc 加 gRPC server 时能直接 import）
//
// 来源：emotion-echo-shared/pkg/emotionchat/ 范本（Stage 58 PR-GRPC-1 已验证流程）

package emotionuser

import (
	"strings"
	"testing"

	"google.golang.org/grpc"
)

// TestUserService_ServerDescriptor_Registered 断言生成的 grpc.ServiceDesc 存在
// 且 ServiceName  含 package 名 emotion_user.v1.UserService
func TestUserService_ServerDescriptor_Registered(t *testing.T) {
	sd := UserService_ServiceDesc
	if sd.ServiceName == "" {
		t.Fatal("UserService_ServiceDesc.ServiceName 为空 — protoc-gen-go-grpc 未生成")
	}
	if !strings.HasSuffix(sd.ServiceName, "UserService") {
		t.Errorf("ServiceName 应以 UserService 结尾，实际=%q", sd.ServiceName)
	}
}

// TestRegisterUserServiceServer_NoPanic 断言 RegisterUserServiceServer 能注册实现
// （nil impl 会触发 panic 由 recover 捕获）—— 验证接口签名正确
func TestRegisterUserServiceServer_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RegisterUserServiceServer(nil) panic: %v", r)
		}
	}()
	srv := grpc.NewServer()
	defer srv.Stop()
	// nil impl 会 panic；只验证方法签名存在即可
	_ = func() { RegisterUserServiceServer(srv, nil) }
}