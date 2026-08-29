package deployment

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/pluginmanager"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/noop"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestReconciler_Reconcile(t *testing.T) {
	// Create a fake scheme
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)

	// Create test deployment
	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{
				Name: "test-model-v1",
			},
			Target: &v2pb.DeploymentSpec_MobileSpec{
				MobileSpec: &v2pb.MobileSpec{
					Level: v2pb.DEVICE_INTEGRITY_LEVEL_STANDARD,
				},
			},
		},
		Status: v2pb.DeploymentStatus{
			Stage: v2pb.DEPLOYMENT_STAGE_INVALID,
		},
	}

	// Create fake client with the deployment
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	// Create mock API handler factory using the existing fake handler
	mockFactory := &mockAPIHandlerFactory{
		handler: handler.NewFakeAPIHandler(fakeClient),
	}

	// Create registrar and register the plugin
	registrar := pluginmanager.NewSimpleRegistrar[plugins.Plugin](logr.Discard())
	noOpPlugin := noop.NewNoOpPlugin()
	registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", noOpPlugin)

	// Create reconciler with registrar
	reconciler := NewReconciler(mockFactory, registrar)
	reconciler.log = logr.Discard()
	reconciler.recorder = &record.FakeRecorder{}

	// Set up with fake manager data
	reconciler.Handler = mockFactory.handler

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: ktypes.NamespacedName{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run reconcile multiple times until completion or timeout
	maxAttempts := 10
	var result ctrl.Result
	var reconcileErr error
	for i := 0; i < maxAttempts; i++ {
		result, reconcileErr = reconciler.Reconcile(ctx, req)
		assert.NoError(t, reconcileErr)

		// Get the updated deployment to check stage
		var updatedDeployment v2pb.Deployment
		getErr := fakeClient.Get(ctx, req.NamespacedName, &updatedDeployment)
		require.NoError(t, getErr)

		// If we've reached completion, break
		if updatedDeployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
			break
		}

		// If no requeue is requested, something is wrong
		if !result.Requeue {
			t.Fatalf("Reconcile stopped requesting requeue but deployment not complete. Stage: %s", updatedDeployment.Status.Stage)
		}
	}

	// Verify deployment was updated to final state
	var finalDeployment v2pb.Deployment
	finalErr := fakeClient.Get(ctx, req.NamespacedName, &finalDeployment)
	require.NoError(t, finalErr)

	// Verify the deployment is marked as completed
	assert.Equal(t, v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE, finalDeployment.Status.Stage)
	// Note: Message may be cleared by controller's handleStageTransition logic
	assert.NotNil(t, finalDeployment.Status.CurrentRevision)
	assert.Equal(t, "test-model-v1", finalDeployment.Status.CurrentRevision.Name)
}

// TestReconciler_Reconcile_FirstTimeDeployUnhealthyDoesNotRollback covers the bug fixed in
// processPlugin: a deployment with no prior successful revision (Status.CurrentRevision == nil)
// must not be routed into rollback just because HealthCheckGate reports unhealthy -- nothing has
// been provisioned yet, so "unhealthy" is expected, not a regression. Before the fix, this always
// dead-ended at DEPLOYMENT_STAGE_ROLLBACK_FAILED because there was nothing in the model-config to
// roll back from.
func TestReconciler_Reconcile_FirstTimeDeployUnhealthyDoesNotRollback(t *testing.T) {
	scheme := runtime.NewScheme()
	err := v2pb.AddToScheme(scheme)
	require.NoError(t, err)

	deployment := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment-unhealthy",
			Namespace: "test-namespace",
		},
		Spec: v2pb.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{
				Name: "test-model-v1",
			},
			Target: &v2pb.DeploymentSpec_MobileSpec{
				MobileSpec: &v2pb.MobileSpec{
					Level: v2pb.DEVICE_INTEGRITY_LEVEL_STANDARD,
				},
			},
		},
		Status: v2pb.DeploymentStatus{
			Stage: v2pb.DEPLOYMENT_STAGE_INVALID,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deployment).
		Build()

	mockFactory := &mockAPIHandlerFactory{
		handler: handler.NewFakeAPIHandler(fakeClient),
	}

	registrar := pluginmanager.NewSimpleRegistrar[plugins.Plugin](logr.Discard())
	registrar.RegisterPlugin(v2pb.TARGET_TYPE_INFERENCE_SERVER.String(), "", &alwaysUnhealthyPlugin{NoOpPlugin: noop.NewNoOpPlugin()})

	reconciler := NewReconciler(mockFactory, registrar)
	reconciler.log = logr.Discard()
	reconciler.recorder = &record.FakeRecorder{}
	reconciler.Handler = mockFactory.handler

	req := ctrl.Request{
		NamespacedName: ktypes.NamespacedName{
			Name:      "test-deployment-unhealthy",
			Namespace: "test-namespace",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		_, reconcileErr := reconciler.Reconcile(ctx, req)
		assert.NoError(t, reconcileErr)

		var updatedDeployment v2pb.Deployment
		getErr := fakeClient.Get(ctx, req.NamespacedName, &updatedDeployment)
		require.NoError(t, getErr)

		require.NotEqual(t, v2pb.DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS, updatedDeployment.Status.Stage,
			"a first-time deploy with no prior revision must not be routed into rollback just because the health check reports unhealthy")
		require.NotEqual(t, v2pb.DEPLOYMENT_STAGE_ROLLBACK_FAILED, updatedDeployment.Status.Stage,
			"a first-time deploy with no prior revision must not be routed into rollback just because the health check reports unhealthy")

		if updatedDeployment.Status.Stage == v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
			break
		}
	}

	var finalDeployment v2pb.Deployment
	finalErr := fakeClient.Get(ctx, req.NamespacedName, &finalDeployment)
	require.NoError(t, finalErr)
	assert.Equal(t, v2pb.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE, finalDeployment.Status.Stage)
}

// alwaysUnhealthyPlugin wraps plugins.Plugin and overrides HealthCheckGate to always report
// unhealthy, simulating a brand-new InferenceServer with no pods provisioned yet.
type alwaysUnhealthyPlugin struct {
	NoOpPlugin plugins.Plugin
}

func (p *alwaysUnhealthyPlugin) HealthCheckGate(ctx context.Context, observability plugins.ObservabilityContext, modelDeployment *v2pb.Deployment) (bool, error) {
	return false, nil
}

func (p *alwaysUnhealthyPlugin) GetState(ctx context.Context, observability plugins.ObservabilityContext, modelDeployment *v2pb.Deployment) (v2pb.DeploymentStatus, error) {
	return p.NoOpPlugin.GetState(ctx, observability, modelDeployment)
}

func (p *alwaysUnhealthyPlugin) GetRolloutPlugin(ctx context.Context, resource *v2pb.Deployment) (conditionInterfaces.Plugin[*v2pb.Deployment], error) {
	return p.NoOpPlugin.GetRolloutPlugin(ctx, resource)
}

func (p *alwaysUnhealthyPlugin) GetRollbackPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.NoOpPlugin.GetRollbackPlugin()
}

func (p *alwaysUnhealthyPlugin) GetCleanupPlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.NoOpPlugin.GetCleanupPlugin()
}

func (p *alwaysUnhealthyPlugin) GetSteadyStatePlugin() conditionInterfaces.Plugin[*v2pb.Deployment] {
	return p.NoOpPlugin.GetSteadyStatePlugin()
}

func (p *alwaysUnhealthyPlugin) ParseStage(resource *v2pb.Deployment) v2pb.DeploymentStage {
	return p.NoOpPlugin.ParseStage(resource)
}

func (p *alwaysUnhealthyPlugin) PopulateDeploymentLogs(ctx context.Context, runtimeContext plugins.RequestContext, modelDeployment *v2pb.Deployment) {
	p.NoOpPlugin.PopulateDeploymentLogs(ctx, runtimeContext, modelDeployment)
}

func (p *alwaysUnhealthyPlugin) PopulateMessage(ctx context.Context, runtimeContext plugins.RequestContext, modelDeployment *v2pb.Deployment) {
	p.NoOpPlugin.PopulateMessage(ctx, runtimeContext, modelDeployment)
}

// Mock implementations
type mockAPIHandlerFactory struct {
	handler api.Handler
}

func (m *mockAPIHandlerFactory) GetAPIHandler(client client.Client) (api.Handler, error) {
	return m.handler, nil
}
