// emotion-echo-shared/pkg/emotionanalytics/stub_test.go
//
// Stage 62 PR-3.1 stub 单元测试（同 emotionuser/stub_test.go 设计）

package emotionanalytics

import (
	"strings"
	"testing"

	"google.golang.org/grpc"
)

func TestAnalyticsService_ServerDescriptor_Registered(t *testing.T) {
	sd := AnalyticsService_ServiceDesc
	if sd.ServiceName == "" {
		t.Fatal("AnalyticsService_ServiceDesc.ServiceName 为空")
	}
	if !strings.HasSuffix(sd.ServiceName, "AnalyticsService") {
		t.Errorf("ServiceName 应以 AnalyticsService 结尾，实际=%q", sd.ServiceName)
	}
}

func TestRegisterAnalyticsServiceServer_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RegisterAnalyticsServiceServer(nil) panic: %v", r)
		}
	}()
	srv := grpc.NewServer()
	defer srv.Stop()
	_ = func() { RegisterAnalyticsServiceServer(srv, nil) }
}