package triggerrun

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Manager provides operations for managing TriggerRun resources
// associated with a pipeline during cascade delete.
type Manager interface {
	ListTriggerRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.TriggerRun, error)
	ListActiveTriggerRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.TriggerRun, error)
	KillTriggerRun(ctx context.Context, tr *v2pb.TriggerRun) error
	DeleteAllTriggerRuns(ctx context.Context, namespace, pipelineName string) error
}

type managerImpl struct {
	k8sClient client.Client
	logger    *zap.Logger
}

// NewManager creates a new TriggerRun Manager backed by the controller-runtime
// client. The controller-runtime client is built with the controllermgr's
// custom scheme (which has v2 types registered), so it does not suffer the
// global-scheme mismatch that the apiserver-shaped api.Handler has when used
// from a controller process.
func NewManager(k8sClient client.Client, logger *zap.Logger) Manager {
	return &managerImpl{k8sClient: k8sClient, logger: logger}
}

// ListTriggerRunsForPipeline returns every TriggerRun in the namespace whose
// spec.pipeline references the given pipeline. Filtering is done in-memory
// because custom FieldSelectors are not supported uniformly across the
// metadata storage backend and the controller-runtime cached client.
func (m *managerImpl) ListTriggerRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.TriggerRun, error) {
	list := &v2pb.TriggerRunList{}
	if err := m.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list trigger runs for pipeline %s/%s: %w", namespace, pipelineName, err)
	}
	var result []*v2pb.TriggerRun
	for i := range list.Items {
		tr := &list.Items[i]
		if tr.Spec.Pipeline == nil {
			continue
		}
		if tr.Spec.Pipeline.Name != pipelineName || tr.Spec.Pipeline.Namespace != namespace {
			continue
		}
		result = append(result, tr)
	}
	return result, nil
}

func (m *managerImpl) ListActiveTriggerRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.TriggerRun, error) {
	all, err := m.ListTriggerRunsForPipeline(ctx, namespace, pipelineName)
	if err != nil {
		return nil, err
	}
	var active []*v2pb.TriggerRun
	for _, tr := range all {
		if !IsTerminateState(tr) {
			active = append(active, tr)
		}
	}
	return active, nil
}

// KillTriggerRun requests termination of the TriggerRun. Sets both the new
// Spec.Action=KILL field and the deprecated Spec.Kill bool so the TR
// controller reacts regardless of the TR's current state (the INVALID-state
// branch in the TR controller only reads the deprecated Spec.Kill field).
func (m *managerImpl) KillTriggerRun(ctx context.Context, tr *v2pb.TriggerRun) error {
	if IsTerminateState(tr) {
		return nil
	}
	tr.Spec.Action = v2pb.TRIGGER_RUN_ACTION_KILL
	tr.Spec.Kill = true
	if err := m.k8sClient.Update(ctx, tr); err != nil {
		return fmt.Errorf("kill trigger run %s/%s: %w", tr.Namespace, tr.Name, err)
	}
	return nil
}

func (m *managerImpl) DeleteAllTriggerRuns(ctx context.Context, namespace, pipelineName string) error {
	triggerRuns, err := m.ListTriggerRunsForPipeline(ctx, namespace, pipelineName)
	if err != nil {
		return err
	}
	for _, tr := range triggerRuns {
		if err := m.k8sClient.Delete(ctx, tr); err != nil {
			if apiErrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("delete trigger run %s/%s: %w", tr.Namespace, tr.Name, err)
		}
	}
	return nil
}
