package creation

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends"
	backendsmocks "github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/backends/backendsmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/inferenceserver/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// createValidationTestRegistry creates a registry with the mock backend registered for Triton.
func createValidationTestRegistry(mockBackend *backendsmocks.MockBackend) *backends.Registry {
	registry := backends.NewRegistry()
	registry.Register(v2pb.BACKEND_TYPE_TRITON, mockBackend)
	return registry
}

func TestValidationActor_Retrieve(t *testing.T) {
	tests := []struct {
		name            string
		backendType     v2pb.BackendType
		clusterTargets  []*v2pb.ClusterTarget
		annotations     map[string]string
		servingImage    *v2pb.ServingImage
		expectedStatus  apipb.ConditionStatus
		expectedReason  string
		expectedMessage string
		expectedErr     bool
	}{
		{
			name:            "valid triton backend type",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
			expectedReason:  "",
			expectedMessage: "",
			expectedErr:     false,
		},
		{
			name:            "invalid backend type - llm-d",
			backendType:     v2pb.BACKEND_TYPE_LLM_D,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidBackendType",
			expectedReason:  "unsupported backend type: BACKEND_TYPE_LLM_D",
			expectedErr:     false,
		},
		{
			name:            "invalid backend type",
			backendType:     v2pb.BACKEND_TYPE_INVALID,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidBackendType",
			expectedReason:  "unsupported backend type: BACKEND_TYPE_INVALID",
			expectedErr:     false,
		},
		{
			name:            "no cluster targets",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "NoClusterTargets",
			expectedReason:  "spec.cluster_targets must declare at least one cluster",
			expectedErr:     false,
		},
		{
			name:            "unknown rollout strategy annotation",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			annotations:     map[string]string{common.ClusterRolloutStrategyAnnotation: "blast"},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidRolloutStrategy",
			expectedReason:  `unknown cluster rollout strategy "blast"; supported: rolling`,
			expectedErr:     false,
		},
		{
			name:           "custom serving image with explicit tag",
			backendType:    v2pb.BACKEND_TYPE_TRITON,
			clusterTargets: []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			servingImage:   &v2pb.ServingImage{Source: &v2pb.ServingImage_Uri{Uri: "myreg.io/team/triton:24.12-py3"}},
			expectedStatus: apipb.CONDITION_STATUS_TRUE,
			expectedErr:    false,
		},
		{
			name:            "serving image without a tag",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			servingImage:    &v2pb.ServingImage{Source: &v2pb.ServingImage_Uri{Uri: "myreg.io/team/triton"}},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidServingImage",
			expectedReason:  `spec.initSpec.servingSpec.image.uri "myreg.io/team/triton": must include an explicit tag or digest`,
			expectedErr:     false,
		},
		{
			name:            "serving image with unsupported pull policy",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			servingImage:    &v2pb.ServingImage{ImagePullPolicy: "Sometimes"},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidServingImage",
			expectedReason:  `spec.initSpec.servingSpec.image.imagePullPolicy "Sometimes": must be one of "Always", "IfNotPresent", or "Never"`,
			expectedErr:     false,
		},
		{
			name:            "serving image with empty pull secret name",
			backendType:     v2pb.BACKEND_TYPE_TRITON,
			clusterTargets:  []*v2pb.ClusterTarget{{ClusterId: "c1"}},
			servingImage:    &v2pb.ServingImage{ImagePullSecrets: []string{"regcred", " "}},
			expectedStatus:  apipb.CONDITION_STATUS_FALSE,
			expectedMessage: "InvalidServingImage",
			expectedReason:  "spec.initSpec.servingSpec.image.imagePullSecrets must not contain empty names",
			expectedErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBackend := backendsmocks.NewMockBackend(ctrl)
			registry := createValidationTestRegistry(mockBackend)
			// No expectations set, backend should not be called

			actor := NewValidationActor(registry, zap.NewNop())

			resource := &v2pb.InferenceServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-server",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
				Spec: v2pb.InferenceServerSpec{
					BackendType:    tt.backendType,
					ClusterTargets: tt.clusterTargets,
					InitSpec: &v2pb.InitSpec{
						ServingSpec: &v2pb.ServingSpec{Image: tt.servingImage},
					},
				},
			}

			condition := &apipb.Condition{
				Type: "TritonValidation",
			}

			result, err := actor.Retrieve(context.Background(), resource, condition)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, result.Status)
				assert.Equal(t, tt.expectedReason, result.Reason)
				assert.Equal(t, tt.expectedMessage, result.Message)
				assert.Equal(t, "TritonValidation", result.Type)
			}
		})
	}
}

func TestValidateImageURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr string
	}{
		{name: "tagged image", uri: "nvcr.io/nvidia/tritonserver:25.01-py3"},
		{name: "digest pinned image", uri: "myreg.io/team/triton@sha256:6f1a"},
		{name: "registry port with tag", uri: "localhost:5000/team/triton:25.01"},
		{name: "docker hub short form with tag", uri: "tritonserver:25.01-py3"},
		// The colon here is a registry port, not a tag.
		{name: "registry port without tag", uri: "localhost:5000/team/triton", wantErr: "must include an explicit tag or digest"},
		{name: "no tag", uri: "nvcr.io/nvidia/tritonserver", wantErr: "must include an explicit tag or digest"},
		{name: "empty tag", uri: "nvcr.io/nvidia/tritonserver:", wantErr: `tag is empty after ":"`},
		{name: "empty digest", uri: "myreg.io/team/triton@", wantErr: `digest is empty after "@"`},
		{name: "embedded whitespace", uri: "myreg.io/team/triton :25.01", wantErr: "must not contain whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImageURI(tt.uri)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
		})
	}
}

func TestValidateServingImageAllowsUnsetImage(t *testing.T) {
	assert.NoError(t, validateServingImage(nil))
	// A pull policy without a URI applies to the default image.
	assert.NoError(t, validateServingImage(&v2pb.ServingImage{ImagePullPolicy: "Always"}))
}

func TestValidationActor_Run(t *testing.T) {
	// Run() simply returns the input condition as-is (no changes).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBackend := backendsmocks.NewMockBackend(ctrl)
	registry := createValidationTestRegistry(mockBackend)
	// No expectations set, backend should not be called

	actor := NewValidationActor(registry, zap.NewNop())

	resource := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-namespace",
		},
		Spec: v2pb.InferenceServerSpec{
			BackendType: v2pb.BACKEND_TYPE_TRITON,
		},
	}

	// Provide an input condition with specific values
	condition := &apipb.Condition{
		Type:    "TritonValidation",
		Status:  apipb.CONDITION_STATUS_FALSE,
		Reason:  "TestReason",
		Message: "TestMessage",
	}

	result, err := actor.Run(context.Background(), resource, condition)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, apipb.CONDITION_STATUS_FALSE, result.Status)
	assert.Equal(t, "TestReason", result.Reason)
	assert.Equal(t, "TestMessage", result.Message)
	assert.Equal(t, "TritonValidation", result.Type)
}
