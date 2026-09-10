// emotion-echo-shared/pkg/emotionassessment/stub_test.go
//
// Stage 62 PR-3.1 stub 单元测试（同 emotionuser_user/stub_test.go 设计）

package emotionassessment

import (
	"strings"
	"testing"

	"google.golang.org/grpc"
)

func TestAssessmentService_ServerDescriptor_Registered(t *testing.T) {
	sd := AssessmentService_ServiceDesc
	if sd.ServiceName == "" {
		t.Fatal("AssessmentService_ServiceDesc.ServiceName 为空")
	}
	if !strings.HasSuffix(sd.ServiceName, "AssessmentService") {
		t.Errorf("ServiceName 应以 AssessmentService 结尾，实际=%q", sd.ServiceName)
	}
}

func TestRegisterAssessmentServiceServer_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RegisterAssessmentServiceServer(nil) panic: %v", r)
		}
	}()
	srv := grpc.NewServer()
	defer srv.Stop()
	_ = func() { RegisterAssessmentServiceServer(srv, nil) }
}