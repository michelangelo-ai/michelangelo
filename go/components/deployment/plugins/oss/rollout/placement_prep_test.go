package rollout

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/oss/common"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testNamespace      = "default"
	testISName         = "test-is"
	testDeploymentName = "test-deployment"
)

// newPlacementFixture builds a PlacementPrepActor wired to a fake kube client preloaded with
// the supplied InferenceServer (or none, to simulate a missing IS).
func newPlacementFixture(t *testing.T, is *v2pb.InferenceServer) (*PlacementPrepActor, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))

	builder := fake.NewClientBuilder().WithScheme(scheme)
	if is != nil {
		builder = builder.WithObjects(is)
	}
	kubeClient := builder.Build()

	actor := &PlacementPrepActor{kubeClient: kubeClient, logger: zap.NewNop()}
	return actor, kubeClient
}

// inferenceServer builds an IS with the given spec ClusterTargets and per-cluster status states.
// `serving` is the set of cluster IDs whose status state is SERVING; the rest get CREATING.
func inferenceServer(specClusters []string, serving []string) *v2pb.InferenceServer {
	is := &v2pb.InferenceServer{
		ObjectMeta: metav1.ObjectMeta{Name: testISName, Namespace: testNamespace},
		Spec:       v2pb.InferenceServerSpec{BackendType: v2pb.BACKEND_TYPE_DYNAMO},
	}
	for _, id := range specClusters {
		is.Spec.ClusterTargets = append(is.Spec.ClusterTargets, &v2pb.ClusterTarget{ClusterId: id})
	}
	servingSet := make(map[string]bool, len(serving))
	for _, id := range serving {
		servingSet[id] = true
	}
	for _, id := range specClusters {
		state := v2pb.INFERENCE_SERVER_STATE_CREATING
		if servingSet[id] {
			state = v2pb.INFERENCE_SERVER_STATE_SERVING
		}
		is.Status.ClusterStatuses = append(is.Status.ClusterStatuses, &v2pb.ClusterTargetStatus{
			ClusterId: id,
			State:     state,
		})
	}
	return is
}

// deploymentWithAnnotation builds a Deployment that references testISName, optionally with a
// pre-set target-clusters annotation value (raw JSON string).
func deploymentWithAnnotation(annotationValue string) *v2pb.Deployment {
	dep := &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: testDeploymentName, Namespace: testNamespace},
		Spec: v2pb.DeploymentSpec{
			Target: &v2pb.DeploymentSpec_InferenceServer{
				InferenceServer: &apipb.ResourceIdentifier{Name: testISName},
			},
		},
	}
	if annotationValue != "" {
		dep.Annotations = map[string]string{common.TargetClustersAnnotation: annotationValue}
	}
	return dep
}

func TestPlacementPrepActor_Retrieve(t *testing.T) {
	tests := []struct {
		name              string
		annotationValue   string // raw JSON; empty means no annotation
		seedIS            *v2pb.InferenceServer
		expectedStatus    apipb.ConditionStatus
		expectedReasonSub string // substring match against condition.Reason
	}{
		{
			name:              "annotation absent",
			annotationValue:   "",
			seedIS:            inferenceServer([]string{"c1"}, []string{"c1"}),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "annotation is absent",
		},
		{
			name:              "annotation invalid JSON",
			annotationValue:   "not json",
			seedIS:            inferenceServer([]string{"c1"}, []string{"c1"}),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "unmarshal",
		},
		{
			name:              "inference server fetch fails",
			annotationValue:   `[{"clusterId":"c1"}]`,
			seedIS:            nil,
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "get inference server",
		},
		{
			name:            "snapshot matches healthy",
			annotationValue: `[{"clusterId":"c1"},{"clusterId":"c2"}]`,
			seedIS:          inferenceServer([]string{"c1", "c2"}, []string{"c1", "c2"}),
			expectedStatus:  apipb.CONDITION_STATUS_TRUE,
		},
		{
			name:              "cluster recovered (more healthy than snapshot)",
			annotationValue:   `[{"clusterId":"c1"}]`,
			seedIS:            inferenceServer([]string{"c1", "c2"}, []string{"c1", "c2"}),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "serving cluster set has changed",
		},
		{
			name:              "cluster degraded (fewer healthy than snapshot)",
			annotationValue:   `[{"clusterId":"c1"},{"clusterId":"c2"}]`,
			seedIS:            inferenceServer([]string{"c1", "c2"}, []string{"c1"}),
			expectedStatus:    apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub: "serving cluster set has changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor, _ := newPlacementFixture(t, tt.seedIS)
			deployment := deploymentWithAnnotation(tt.annotationValue)

			got, err := actor.Retrieve(context.Background(), deployment, &apipb.Condition{})

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
		})
	}
}

func TestPlacementPrepActor_Run(t *testing.T) {
	tests := []struct {
		name                string
		annotationValue     string
		seedIS              *v2pb.InferenceServer
		expectedStatus      apipb.ConditionStatus
		expectedReasonSub   string
		expectedAnnotation  bool // whether the deployment annotation should be present after Run
		expectErrorReturned bool // whether Run returned a non-nil error
	}{
		{
			name:                "inference server fetch fails",
			annotationValue:     "",
			seedIS:              nil,
			expectedStatus:      apipb.CONDITION_STATUS_FALSE,
			expectedReasonSub:   "get inference server",
			expectErrorReturned: true,
		},
		{
			name:               "no cluster serving",
			annotationValue:    "",
			seedIS:             inferenceServer([]string{"c1"}, nil),
			expectedStatus:     apipb.CONDITION_STATUS_UNKNOWN,
			expectedReasonSub:  "no cluster has reached serving state",
			expectedAnnotation: false,
		},
		{
			name:               "first write (annotation absent)",
			annotationValue:    "",
			seedIS:             inferenceServer([]string{"c1"}, []string{"c1"}),
			expectedStatus:     apipb.CONDITION_STATUS_UNKNOWN,
			expectedReasonSub:  "actor chain rebuilds on next reconcile",
			expectedAnnotation: true,
		},
		{
			name:               "drift refresh (annotation present, healthy set differs)",
			annotationValue:    `[{"clusterId":"c1"}]`,
			seedIS:             inferenceServer([]string{"c1", "c2"}, []string{"c1", "c2"}),
			expectedStatus:     apipb.CONDITION_STATUS_TRUE,
			expectedAnnotation: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor, _ := newPlacementFixture(t, tt.seedIS)
			deployment := deploymentWithAnnotation(tt.annotationValue)

			got, err := actor.Run(context.Background(), deployment, &apipb.Condition{})

			if tt.expectErrorReturned {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectedStatus, got.Status)
			if tt.expectedReasonSub != "" {
				assert.Contains(t, got.Reason, tt.expectedReasonSub)
			}
			if tt.expectedAnnotation {
				assert.Contains(t, deployment.Annotations, common.TargetClustersAnnotation)
				assert.Equal(t, v2pb.BACKEND_TYPE_DYNAMO, common.BackendTypeOf(deployment),
					"the inference server's backend is snapshotted next to the cluster set")
			}
		})
	}
}
