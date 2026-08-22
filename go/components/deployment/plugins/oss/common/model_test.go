package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	goapi "github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/api/handler"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testModelName   = "model-v1"
	testOtherNS     = "other-ns"
	testArtifactURI = "s3://bucket/artifacts/model-v1/"
)

func newModelClient(t *testing.T, objects ...client.Object) goapi.Handler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v2pb.AddToScheme(scheme))
	return handler.NewFakeAPIHandler(fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build())
}

func newModel(namespace string, packageType v2pb.DeployableModelPackageType, uris ...string) *v2pb.Model {
	return &v2pb.Model{
		ObjectMeta: metav1.ObjectMeta{Name: testModelName, Namespace: namespace},
		Spec: v2pb.ModelSpec{
			PackageType:           packageType,
			DeployableArtifactUri: uris,
		},
	}
}

func newModelDeployment(revision *apipb.ResourceIdentifier) *v2pb.Deployment {
	return &v2pb.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: testNamespace},
		Spec:       v2pb.DeploymentSpec{DesiredRevision: revision},
	}
}

func TestFetchModel(t *testing.T) {
	tritonModel := newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, testArtifactURI)
	otherNSModel := newModel(testOtherNS, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, testArtifactURI)

	tests := []struct {
		name          string
		revision      *apipb.ResourceIdentifier
		objects       []client.Object
		wantNamespace string
		wantReason    string
	}{
		{
			name:          "namespace defaults to the deployment namespace",
			revision:      &apipb.ResourceIdentifier{Name: testModelName},
			objects:       []client.Object{tritonModel},
			wantNamespace: testNamespace,
		},
		{
			name:          "explicit namespace is honoured",
			revision:      &apipb.ResourceIdentifier{Name: testModelName, Namespace: testOtherNS},
			objects:       []client.Object{otherNSModel},
			wantNamespace: testOtherNS,
		},
		{
			name:       "nil revision",
			revision:   nil,
			wantReason: ReasonModelNotFound,
		},
		{
			name:       "empty revision name",
			revision:   &apipb.ResourceIdentifier{},
			wantReason: ReasonModelNotFound,
		},
		{
			name:       "model absent from the cluster",
			revision:   &apipb.ResourceIdentifier{Name: testModelName},
			wantReason: ReasonModelNotFound,
		},
		{
			name:       "model exists only in another namespace",
			revision:   &apipb.ResourceIdentifier{Name: testModelName},
			objects:    []client.Object{otherNSModel},
			wantReason: ReasonModelNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FetchModel(context.Background(), newModelClient(t, tt.objects...), newModelDeployment(tt.revision))

			if tt.wantReason != "" {
				require.Error(t, err)
				var resErr *ModelResolutionError
				require.ErrorAs(t, err, &resErr)
				assert.Equal(t, tt.wantReason, resErr.Reason)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, testModelName, got.Name)
			assert.Equal(t, tt.wantNamespace, got.Namespace)
		})
	}
}

func TestResolveModelStoragePath(t *testing.T) {
	tests := []struct {
		name       string
		model      *v2pb.Model
		wantPath   string
		wantReason string
	}{
		{
			name:     "triton model with a prefix URI",
			model:    newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, testArtifactURI),
			wantPath: testArtifactURI,
		},
		{
			name:     "missing trailing slash is normalised",
			model:    newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "s3://bucket/artifacts/model-v1"),
			wantPath: "s3://bucket/artifacts/model-v1/",
		},
		{
			name:     "first URI wins when several are listed",
			model:    newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, testArtifactURI, "s3://bucket/other/"),
			wantPath: testArtifactURI,
		},
		{
			name:       "package type must be triton",
			model:      newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_SPARK_PIPELINE, testArtifactURI),
			wantReason: ReasonModelPackageTypeMismatch,
		},
		{
			// The registration path never sets package_type, so unset must stay deployable.
			name:     "unset package type defaults to triton",
			model:    newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_INVALID, testArtifactURI),
			wantPath: testArtifactURI,
		},
		{
			name:     "tar artifact keeps its exact key",
			model:    newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "s3://bucket/models/m/abc/deployable/m/__dir__.tar"),
			wantPath: "s3://bucket/models/m/abc/deployable/m/__dir__.tar",
		},
		{
			name:       "no deployable artifact URI",
			model:      newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON),
			wantReason: ReasonNoDeployableArtifact,
		},
		{
			name:       "gcs URIs are not supported by the sidecar",
			model:      newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, "gs://bucket/artifacts/model-v1/"),
			wantReason: ReasonUnsupportedArtifactScheme,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveModelStoragePath(tt.model)

			if tt.wantReason != "" {
				require.Error(t, err)
				var resErr *ModelResolutionError
				require.ErrorAs(t, err, &resErr)
				assert.Equal(t, tt.wantReason, resErr.Reason)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

func TestResolveDeploymentModelStoragePath(t *testing.T) {
	c := newModelClient(t, newModel(testNamespace, v2pb.DEPLOYABLE_MODEL_PACKAGE_TYPE_TRITON, testArtifactURI))

	got, err := ResolveDeploymentModelStoragePath(context.Background(), c,
		newModelDeployment(&apipb.ResourceIdentifier{Name: testModelName}))

	require.NoError(t, err)
	assert.Equal(t, testArtifactURI, got)
}
