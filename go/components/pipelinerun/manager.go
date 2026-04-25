package pipelinerun

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// Manager provides operations for managing PipelineRun resources
// associated with a pipeline during cascade delete.
type Manager interface {
	ListPipelineRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.PipelineRun, error)
	ListActivePipelineRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.PipelineRun, error)
	KillPipelineRun(ctx context.Context, pr *v2pb.PipelineRun) error
}

type managerImpl struct {
	k8sClient client.Client
	logger    *zap.Logger
}

// NewManager creates a new PipelineRun Manager backed by the controller-runtime
// client. The controller-runtime client is built with the controllermgr's
// custom scheme (which has v2 types registered), so it does not suffer the
// global-scheme mismatch that the apiserver-shaped api.Handler has when used
// from a controller process.
func NewManager(k8sClient client.Client, logger *zap.Logger) Manager {
	return &managerImpl{k8sClient: k8sClient, logger: logger}
}

// ListPipelineRunsForPipeline returns every PipelineRun in the namespace whose
// spec.pipeline references the given pipeline. Filtering is done in-memory
// because custom FieldSelectors are not supported uniformly across the
// metadata storage backend and the controller-runtime cached client.
func (m *managerImpl) ListPipelineRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.PipelineRun, error) {
	list := &v2pb.PipelineRunList{}
	if err := m.k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("list pipeline runs for pipeline %s/%s: %w", namespace, pipelineName, err)
	}
	var result []*v2pb.PipelineRun
	for i := range list.Items {
		pr := &list.Items[i]
		if pr.Spec.Pipeline == nil {
			continue
		}
		if pr.Spec.Pipeline.Name != pipelineName || pr.Spec.Pipeline.Namespace != namespace {
			continue
		}
		result = append(result, pr)
	}
	return result, nil
}

func (m *managerImpl) ListActivePipelineRunsForPipeline(ctx context.Context, namespace, pipelineName string) ([]*v2pb.PipelineRun, error) {
	all, err := m.ListPipelineRunsForPipeline(ctx, namespace, pipelineName)
	if err != nil {
		return nil, err
	}
	var active []*v2pb.PipelineRun
	for _, pr := range all {
		if !IsTerminalState(pr.Status.State) {
			active = append(active, pr)
		}
	}
	return active, nil
}

func (m *managerImpl) KillPipelineRun(ctx context.Context, pr *v2pb.PipelineRun) error {
	if IsTerminalState(pr.Status.State) {
		return nil
	}
	pr.Spec.Kill = true
	if err := m.k8sClient.Update(ctx, pr); err != nil {
		return fmt.Errorf("kill pipeline run %s/%s: %w", pr.Namespace, pr.Name, err)
	}
	return nil
}
