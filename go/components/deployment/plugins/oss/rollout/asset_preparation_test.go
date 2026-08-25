package rollout

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	plugincommon "github.com/michelangelo-ai/michelangelo/go/components/deployment/plugins/common"
	"github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	assetModelName = "bert_cola"
	assetNamespace = "default"
)

// newAssetClient builds a control-plane client seeded with the supplied objects.
func newAssetClient(t *testing.T, objects ...client.Object) goapi.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return handler.NewFakeAPIHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())
}

// assetModel returns a Triton-packaged Model CR, optionally overridden per test.
func assetModel(packageType v2pb.DeployableModelPackageType, uris ...string) *v2pb.Model {
	return &v2pb.Model{
		ObjectMeta: metav1.ObjectMeta{Name: assetModelName, Namespace: assetNamespace},
		Spec: v2pb.ModelSpec{
			PackageType:           packageType,
			DeployableArtifactUri: uris,
		},
	}
}

// assetDeployment references assetModelName, leaving the namespace unset so the
// Deployment's own namespace is used.
func assetDeployment(revision *api.ResourceIdentifier) *v2pb.Deployment {
	return &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment", Namespace: assetNamespace},
		Spec:       v2pb.DeploymentSpec{DesiredRevision: revision},
	}
}

func TestAssetPreparationRetrieve(t *testing.T) {
	tests := []struct {
		name                    string
		deployment              *v2pb.Deployment
		objects                 []client.Object
		expectedConditionStatus api.ConditionStatus
		// expectedCode is the short identifier; the repo's condition helper stores it on
		// Message, with the human-readable text on Reason.
		expectedCode string
	}{
		{
			name:                    "assets available when the model resolves",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			objects:                 []client.Object{assetModel(v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "s3://bucket/models/bert_cola/")},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
		},
		{
			name:                    "explicit namespace on the revision resolves",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName, Namespace: assetNamespace}),
			objects:                 []client.Object{assetModel(v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "s3://bucket/models/bert_cola/")},
			expectedConditionStatus: api.CONDITION_STATUS_TRUE,
		},
		{
			name:                    "no desired revision specified",
			deployment:              assetDeployment(nil),
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedCode:            "NoDesiredRevision",
		},
		{
			name:                    "model CR does not exist",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedCode:            plugincommon.ReasonModelNotFound,
		},
		{
			name:                    "model is packaged for another backend",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			objects:                 []client.Object{assetModel(v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE, "s3://bucket/models/bert_cola/")},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedCode:            plugincommon.ReasonModelPackageTypeMismatch,
		},
		{
			name:                    "model has no deployable artifact",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			objects:                 []client.Object{assetModel(v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON)},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedCode:            plugincommon.ReasonNoDeployableArtifact,
		},
		{
			name:                    "artifact URI uses an unsupported scheme",
			deployment:              assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			objects:                 []client.Object{assetModel(v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "gs://bucket/models/bert_cola/")},
			expectedConditionStatus: api.CONDITION_STATUS_FALSE,
			expectedCode:            plugincommon.ReasonUnsupportedArtifactScheme,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &AssetPreparationActor{
				apiHandler: newAssetClient(t, tt.objects...),
				logger:     zap.NewNop(),
			}

			condition, err := actor.Retrieve(context.Background(), tt.deployment, &api.Condition{})

			assert.NoError(t, err)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionStatus, condition.Status)
			if tt.expectedCode != "" {
				assert.Equal(t, tt.expectedCode, condition.Message)
			}
		})
	}
}

// TestAssetPreparationRetrieveReadFailure covers a read that fails for a reason other than
// the model being absent, which has to be reported as a read failure rather than as one of
// the resolver's model-shaped reasons.
func TestAssetPreparationRetrieveReadFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	apiHandler := apimocks.NewMockHandler(ctrl)
	apiHandler.EXPECT().
		Get(gomock.Any(), assetNamespace, assetModelName, gomock.Any(), gomock.Any()).
		Return(status.Error(codes.Unavailable, "metadata storage unreachable"))

	actor := &AssetPreparationActor{apiHandler: apiHandler, logger: zap.NewNop()}

	condition, err := actor.Retrieve(context.Background(),
		assetDeployment(&api.ResourceIdentifier{Name: assetModelName}), &api.Condition{})

	require.NoError(t, err)
	assert.Equal(t, api.CONDITION_STATUS_FALSE, condition.Status)
	assert.Equal(t, "ModelResolutionFailed", condition.Message)
	assert.Contains(t, condition.Reason, "metadata storage unreachable")
}

func TestAssetPreparationGetType(t *testing.T) {
	actor := &AssetPreparationActor{logger: zap.NewNop()}

	assert.Equal(t, "AssetsPrepared", actor.GetType())
}

func TestAssetPreparationRun(t *testing.T) {
	tests := []struct {
		name           string
		deployment     *v2pb.Deployment
		inputCondition *api.Condition
	}{
		{
			name:       "run returns input condition unchanged",
			deployment: assetDeployment(&api.ResourceIdentifier{Name: assetModelName}),
			inputCondition: &api.Condition{
				Status: api.CONDITION_STATUS_TRUE,
				Reason: "TestReason",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actor := &AssetPreparationActor{
				logger: zap.NewNop(),
			}

			condition, err := actor.Run(context.Background(), tt.deployment, tt.inputCondition)

			assert.NoError(t, err)
			assert.Equal(t, tt.inputCondition, condition)
		})
	}
}
