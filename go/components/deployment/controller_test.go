package deployment

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins"
	"github.com/michelangelo-ai/michelangelo/go/components/deployment/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconciler_Reconcile(t *testing.T) {
	// Create a fake scheme
	scheme := runtime.NewScheme()
	err := types.AddToScheme(scheme)
	require.NoError(t, err)

	// Create test deployment
	deployment := &types.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "test-namespace",
		},
		Spec: types.DeploymentSpec{
			DesiredRevision: &types.ModelRevision{
				Name: "test-model-v1",
			},
			Definition: &types.TargetDefinition{
				Type: types.TARGET_TYPE_INFERENCE_SERVER,
			},
		},
		Status: types.DeploymentStatus{
			Stage: types.DEPLOYMENT_STAGE_VALIDATION,
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

	// Create reconciler
	reconciler := NewReconciler(mockFactory)
	reconciler.Log = logr.Discard()
	reconciler.Recorder = &record.FakeRecorder{}

	// Set up with fake manager data
	reconciler.Handler = mockFactory.handler

	// Register the plugin manually since we're not using SetupWithManager
	noOpPlugin := plugins.NewNoOpPlugin()
	reconciler.Registrar.RegisterPlugin(types.TARGET_TYPE_INFERENCE_SERVER.String(), "", noOpPlugin)

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
		var updatedDeployment types.Deployment
		getErr := fakeClient.Get(ctx, req.NamespacedName, &updatedDeployment)
		require.NoError(t, getErr)

		// If we've reached completion, break
		if updatedDeployment.Status.Stage == types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE {
			break
		}

		// If no requeue is requested, something is wrong
		if !result.Requeue {
			t.Fatalf("Reconcile stopped requesting requeue but deployment not complete. Stage: %s", updatedDeployment.Status.Stage)
		}
	}

	// Verify deployment was updated to final state
	var finalDeployment types.Deployment
	finalErr := fakeClient.Get(ctx, req.NamespacedName, &finalDeployment)
	require.NoError(t, finalErr)

	// Verify the deployment is marked as completed
	assert.Equal(t, types.DEPLOYMENT_STAGE_ROLLOUT_COMPLETE, finalDeployment.Status.Stage)
	// Note: Message may be cleared by controller's handleStageTransition logic
	assert.NotNil(t, finalDeployment.Status.CurrentRevision)
	assert.Equal(t, "test-model-v1", finalDeployment.Status.CurrentRevision.Name)
}

// Mock implementations
type mockAPIHandlerFactory struct {
	handler api.Handler
}

func (m *mockAPIHandlerFactory) GetAPIHandler(client client.Client) (api.Handler, error) {
	return m.handler, nil
}