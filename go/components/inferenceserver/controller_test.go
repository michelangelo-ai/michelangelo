package inferenceserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	conditionInterfacesMocks "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces/interfaces_mock"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

func TestReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name                       string
		inferenceServer            *v2pb.InferenceServer
		setupPlugin                func() (*mockInferenceServerPlugin, *mockPlugin, *mockPlugin, *mockConditionActor, *mockConditionActor)
		registerPlugin             bool
		expectEngineRun            bool
		expectCreationPluginCalled bool
		expectDeletionPluginCalled bool
		expectError                bool
		expectEvent                bool
	}{
		{
			name: "creation plugin is called for normal operations",
			inferenceServer: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
				},
				Status: v2pb.InferenceServerStatus{
					State: v2pb.INFERENCE_SERVER_STATE_CREATING,
				},
			},
			setupPlugin: func() (*mockInferenceServerPlugin, *mockPlugin, *mockPlugin, *mockConditionActor, *mockConditionActor) {
				creationActor := &mockConditionActor{actorType: "TestCreation"}
				creationPlugin := &mockPlugin{
					actors:     []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{creationActor},
					pluginType: "creation",
				}
				backendPlugin := &mockInferenceServerPlugin{
					creationPlugin: creationPlugin,
				}
				return backendPlugin, creationPlugin, nil, creationActor, nil
			},
			registerPlugin:             true,
			expectEngineRun:            true,
			expectCreationPluginCalled: true,
			expectDeletionPluginCalled: false,
			expectError:                false,
			expectEvent:                false,
		},
		{
			name: "deletion plugin is called when deletion timestamp is set",
			inferenceServer: func() *v2pb.InferenceServer {
				now := metav1.NewTime(time.Now())
				return &v2pb.InferenceServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-server",
						Namespace:         "test-namespace",
						DeletionTimestamp: &now,
						Finalizers:        []string{"test-finalizer"},
					},
					Spec: v2pb.InferenceServerSpec{
						BackendType: v2pb.BACKEND_TYPE_TRITON,
					},
					Status: v2pb.InferenceServerStatus{
						State: v2pb.INFERENCE_SERVER_STATE_DELETING,
					},
				}
			}(),
			setupPlugin: func() (*mockInferenceServerPlugin, *mockPlugin, *mockPlugin, *mockConditionActor, *mockConditionActor) {
				deletionActor := &mockConditionActor{actorType: "TestDeletion"}
				deletionPlugin := &mockPlugin{
					actors:     []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{deletionActor},
					pluginType: "deletion",
				}
				backendPlugin := &mockInferenceServerPlugin{
					deletionPlugin: deletionPlugin,
				}
				return backendPlugin, nil, deletionPlugin, nil, deletionActor
			},
			registerPlugin:             true,
			expectEngineRun:            true,
			expectCreationPluginCalled: false,
			expectDeletionPluginCalled: true,
			expectError:                false,
			expectEvent:                false,
		},
		{
			name: "deletion plugin is called when decommission is true",
			inferenceServer: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
					DecomSpec: &v2pb.DecomSpec{
						Decommission: true,
					},
				},
				Status: v2pb.InferenceServerStatus{
					State: v2pb.INFERENCE_SERVER_STATE_DELETING,
				},
			},
			setupPlugin: func() (*mockInferenceServerPlugin, *mockPlugin, *mockPlugin, *mockConditionActor, *mockConditionActor) {
				deletionActor := &mockConditionActor{actorType: "TestDeletion"}
				deletionPlugin := &mockPlugin{
					actors:     []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]{deletionActor},
					pluginType: "deletion",
				}
				backendPlugin := &mockInferenceServerPlugin{
					deletionPlugin: deletionPlugin,
				}
				return backendPlugin, nil, deletionPlugin, nil, deletionActor
			},
			registerPlugin:             true,
			expectEngineRun:            true,
			expectCreationPluginCalled: false,
			expectDeletionPluginCalled: true,
			expectError:                false,
			expectEvent:                false,
		},
		{
			name: "plugin not found error is caught and event recorded",
			inferenceServer: &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "test-namespace",
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType: v2pb.BACKEND_TYPE_TRITON,
				},
			},
			setupPlugin: func() (*mockInferenceServerPlugin, *mockPlugin, *mockPlugin, *mockConditionActor, *mockConditionActor) {
				return nil, nil, nil, nil, nil
			},
			registerPlugin:             false,
			expectEngineRun:            false,
			expectCreationPluginCalled: false,
			expectDeletionPluginCalled: false,
			expectError:                false,
			expectEvent:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gomockCtrl := gomock.NewController(t)
			defer gomockCtrl.Finish()

			// Setup registry and plugins
			registry := newMockPluginRegistry()
			backendPlugin, creationPlugin, deletionPlugin, creationActor, deletionActor := tt.setupPlugin()
			if tt.registerPlugin && backendPlugin != nil {
				registry.RegisterPlugin(v2pb.BACKEND_TYPE_TRITON, backendPlugin)
			}

			// Create fake k8s client with the inference server
			scheme := runtime.NewScheme()
			_ = v2pb.AddToScheme(scheme)
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.inferenceServer).
				Build()

			handler := apiHandler.NewFakeAPIHandler(k8sClient)

			// Setup mock engine
			mockEngine := conditionInterfacesMocks.NewMockEngine[*v2pb.InferenceServer](gomockCtrl)
			if tt.expectEngineRun {
				mockEngine.EXPECT().
					Run(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, plugin conditionInterfaces.Plugin[*v2pb.InferenceServer], resource *v2pb.InferenceServer) (conditionInterfaces.Result, error) {
						// Execute all actors
						for _, actor := range plugin.GetActors() {
							condition := &apipb.Condition{Type: actor.GetType()}
							newCondition, _ := actor.Retrieve(ctx, resource, condition)
							plugin.PutCondition(resource, newCondition)
						}
						return conditionInterfaces.Result{
							Result:       ctrl.Result{},
							AreSatisfied: true,
							IsTerminal:   false,
							IsKilled:     false,
						}, nil
					})
			}

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := &Reconciler{
				Handler:  handler,
				logger:   zap.NewNop(),
				Recorder: fakeRecorder,
				Plugins:  registry,
				engine:   mockEngine,
			}

			// Execute
			result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: "test-namespace",
					Name:      "test-server",
				},
			})

			// Assert
			require.NoError(t, err, "Reconcile should not return error due to production pattern")

			if tt.expectEvent {
				// Check that an event was recorded
				select {
				case event := <-fakeRecorder.Events:
					assert.Contains(t, event, "ReconciliationError")
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected event to be recorded but none was found")
				}
				assert.Equal(t, ActiveRequeueAfter, result.RequeueAfter)
			} else {
				assert.NotZero(t, result.RequeueAfter, "Should requeue")
			}

			if tt.expectCreationPluginCalled {
				assert.True(t, backendPlugin.updateDetailsCalled, "UpdateDetails should be called")
				assert.True(t, backendPlugin.updateConditionsCalled, "UpdateConditions should be called")
				assert.True(t, backendPlugin.parseStateCalled, "ParseState should be called")
				assert.True(t, creationPlugin.getActorsCalled, "Creation plugin GetActors should be called")
				assert.True(t, creationActor.retrieveCalled, "Creation actor Retrieve should be called")
			}

			if tt.expectDeletionPluginCalled {
				assert.True(t, backendPlugin.updateDetailsCalled, "UpdateDetails should be called")
				assert.True(t, backendPlugin.updateConditionsCalled, "UpdateConditions should be called")
				assert.True(t, backendPlugin.parseStateCalled, "ParseState should be called")
				assert.True(t, deletionPlugin.getActorsCalled, "Deletion plugin GetActors should be called")
				assert.True(t, deletionActor.retrieveCalled, "Deletion actor Retrieve should be called")
			}
		})
	}
}

// Mock Plugin implementations to aid with testing

// mockConditionActor tracks whether Run and Retrieve were called
type mockConditionActor struct {
	actorType      string
	runCalled      bool
	retrieveCalled bool
}

func (m *mockConditionActor) GetType() string {
	return m.actorType
}

func (m *mockConditionActor) Retrieve(ctx context.Context, resource *v2pb.InferenceServer, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	m.retrieveCalled = true
	return &apipb.Condition{
		Type:    m.actorType,
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "Success",
		Message: "Condition satisfied",
	}, nil
}

func (m *mockConditionActor) Run(ctx context.Context, resource *v2pb.InferenceServer, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	m.runCalled = true
	return &apipb.Condition{
		Type:    m.actorType,
		Status:  apipb.CONDITION_STATUS_TRUE,
		Reason:  "Success",
		Message: "Action completed",
	}, nil
}

// mockPlugin tracks which actors were used
type mockPlugin struct {
	actors          []conditionInterfaces.ConditionActor[*v2pb.InferenceServer]
	pluginType      string
	getActorsCalled bool
}

func (m *mockPlugin) GetActors() []conditionInterfaces.ConditionActor[*v2pb.InferenceServer] {
	m.getActorsCalled = true
	return m.actors
}

func (m *mockPlugin) GetConditions(resource *v2pb.InferenceServer) []*apipb.Condition {
	return resource.Status.Conditions
}

func (m *mockPlugin) PutCondition(resource *v2pb.InferenceServer, condition *apipb.Condition) {
	// Find and update existing condition or append new one
	for i, c := range resource.Status.Conditions {
		if c.Type == condition.Type {
			resource.Status.Conditions[i] = condition
			return
		}
	}
	resource.Status.Conditions = append(resource.Status.Conditions, condition)
}

// mockInferenceServerPlugin implements the InferenceServerPlugin interface
type mockInferenceServerPlugin struct {
	creationPlugin         *mockPlugin
	deletionPlugin         *mockPlugin
	updateDetailsCalled    bool
	updateConditionsCalled bool
	parseStateCalled       bool
}

func (m *mockInferenceServerPlugin) GetCreationPlugin() conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return m.creationPlugin
}

func (m *mockInferenceServerPlugin) GetDeletionPlugin(resource *v2pb.InferenceServer) conditionInterfaces.Plugin[*v2pb.InferenceServer] {
	return m.deletionPlugin
}

func (m *mockInferenceServerPlugin) ParseState(resource *v2pb.InferenceServer) v2pb.InferenceServerState {
	m.parseStateCalled = true
	return v2pb.INFERENCE_SERVER_STATE_SERVING
}

func (m *mockInferenceServerPlugin) UpdateDetails(ctx context.Context, resource *v2pb.InferenceServer) error {
	m.updateDetailsCalled = true
	return nil
}

func (m *mockInferenceServerPlugin) UpdateConditions(resource *v2pb.InferenceServer, conditionPlugin conditionInterfaces.Plugin[*v2pb.InferenceServer]) {
	m.updateConditionsCalled = true
}

// mockPluginRegistry manages test plugins
type mockPluginRegistry struct {
	plugins map[v2pb.BackendType]plugins.InferenceServerPlugin
}

func newMockPluginRegistry() *mockPluginRegistry {
	return &mockPluginRegistry{
		plugins: make(map[v2pb.BackendType]plugins.InferenceServerPlugin),
	}
}

func (m *mockPluginRegistry) RegisterPlugin(backendType v2pb.BackendType, plugin plugins.InferenceServerPlugin) {
	m.plugins[backendType] = plugin
}

func (m *mockPluginRegistry) GetPlugin(backendType v2pb.BackendType) (plugins.InferenceServerPlugin, error) {
	if plugin, ok := m.plugins[backendType]; ok {
		return plugin, nil
	}
	return nil, fmt.Errorf("no plugin registered for backend type: %v", backendType)
}
