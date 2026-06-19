//go:build sandbox
// +build sandbox

// Sandbox integration tests for the notification subsystem.
//
// Cadence (requires in-cluster Cadence on localhost:7833 and bazel run //go/cmd/worker):
//
//	cd go && GOTOOLCHAIN=auto go test -tags sandbox -v -run '^TestSandboxNotifyOnStateChange$' \
//	  ./components/pipelinerun/notification/... -timeout 20s
//
// Temporal (requires kubectl port-forward svc/michelangelo-temporal-frontend 7233:7233
// and bazel run //go/cmd/worker with WORKFLOW_ENGINE_PROVIDER=temporal):
//
//	cd go && GOTOOLCHAIN=auto go test -tags sandbox -v -run '^TestSandboxNotifyOnStateChangeTemporal$' \
//	  ./components/pipelinerun/notification/... -timeout 20s
package notification

import (
	"context"
	"testing"
	"time"

	baseconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/cadenceclient"
	"github.com/michelangelo-ai/michelangelo/go/base/workflowclient/temporalclient"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSandboxNotifyOnStateChange fires a real notification workflow against the
// in-cluster Cadence. Watch notification-worker logs and
// http://localhost:8088 for workflow execution.
func TestSandboxNotifyOnStateChange(t *testing.T) {
	cfg := baseconfig.WorkflowClientConfig{
		Host:      "localhost:7833",
		Transport: "grpc",
		Domain:    "default",
		TaskList:  "notification_worker",
	}
	out, err := cadenceclient.NewCadenceClient(cadenceclient.CadenceClientIn{Config: cfg})
	require.NoError(t, err, "failed to create Cadence client")

	logger, _ := zap.NewDevelopment()
	notifier, err := NewPipelineRunNotifier(
		Config{
			TaskList:      "notification_worker",
			StudioBaseURL: "http://localhost:3000/studio/",
			SenderEmail:   "",
		},
		out.CadenceClient,
		logger,
	)
	require.NoError(t, err)

	oldRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-notif-run-3", Namespace: "default"},
		Status:     v2pb.PipelineRunStatus{State: v2pb.PIPELINE_RUN_STATE_PENDING},
	}
	newRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-notif-run-3",
			Namespace: "default",
			Labels: map[string]string{
				"michelangelo/SourcePipelineType":            "PIPELINE_TYPE_TRAIN",
				"pipeline.michelangelo/PipelineManifestType": "PIPELINE_MANIFEST_TYPE_ASL",
			},
		},
		Spec: v2pb.PipelineRunSpec{
			Notifications: []*v2pb.Notification{
				{
					EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
					Emails:            []string{"test@example.com"},
					SlackDestinations: []string{"#test-alerts"},
				},
			},
		},
		Status: v2pb.PipelineRunStatus{
			State:  v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			LogUrl: "https://workflow.example.com/run/abc123",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = notifier.NotifyOnStateChange(ctx, oldRun, newRun)
	require.NoError(t, err, "NotifyOnStateChange must start workflow without error")
	t.Log("SUCCESS — workflow started. Check:")
	t.Log("  • worker logs:  bazel run //go/cmd/worker  (stdout of that terminal)")
	t.Log("  • Cadence UI:   http://localhost:8088")
}

// TestSandboxNotifyOnStateChangeTemporal fires a real notification workflow against
// a local Temporal server. Prerequisites:
//
//  1. kubectl port-forward svc/michelangelo-temporal-frontend 7233:7233
//  2. notification-worker running with:
//     WORKFLOW_ENGINE_PROVIDER=temporal WORKFLOW_ENGINE_HOST=127.0.0.1:7233
func TestSandboxNotifyOnStateChangeTemporal(t *testing.T) {
	cfg := baseconfig.WorkflowClientConfig{
		Host:     "localhost:7233",
		Domain:   "default",
		Provider: "Temporal",
	}
	out, err := temporalclient.NewTemporalClient(temporalclient.TemporalClientIn{Config: cfg})
	require.NoError(t, err, "failed to create Temporal client")

	logger, _ := zap.NewDevelopment()
	notifier, err := NewPipelineRunNotifier(
		Config{
			TaskList:      "notification_worker",
			StudioBaseURL: "http://localhost:3000/studio/",
			SenderEmail:   "",
		},
		out.TemporalClient,
		logger,
	)
	require.NoError(t, err)

	oldRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox-notif-temporal-1", Namespace: "default"},
		Status:     v2pb.PipelineRunStatus{State: v2pb.PIPELINE_RUN_STATE_PENDING},
	}
	newRun := &v2pb.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sandbox-notif-temporal-1",
			Namespace: "default",
			Labels: map[string]string{
				"michelangelo/SourcePipelineType":            "PIPELINE_TYPE_TRAIN",
				"pipeline.michelangelo/PipelineManifestType": "PIPELINE_MANIFEST_TYPE_ASL",
			},
		},
		Spec: v2pb.PipelineRunSpec{
			Notifications: []*v2pb.Notification{
				{
					EventTypes:        []v2pb.Notification_EventType{v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED},
					Emails:            []string{"test@example.com"},
					SlackDestinations: []string{"#test-alerts"},
				},
			},
		},
		Status: v2pb.PipelineRunStatus{
			State:  v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
			LogUrl: "https://workflow.example.com/run/abc123",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = notifier.NotifyOnStateChange(ctx, oldRun, newRun)
	require.NoError(t, err, "NotifyOnStateChange must start workflow without error")
	t.Log("SUCCESS — workflow started. Check:")
	t.Log("  • worker logs:  bazel run //go/cmd/worker  (stdout of that terminal)")
	t.Log("  • Temporal UI:  http://localhost:8080")
}
