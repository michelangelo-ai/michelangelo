package testutils

import (
	"testing"

	"code.uber.internal/base/testing/contextmatcher"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/utils"
	"github.com/golang/mock/gomock"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Test constants for default project values
const (
	DefaultTestNamespace = "test-namespace"
	DefaultTestTier      = 0
	DefaultTestTeam      = "test-team"
)

// CreateMockAPIHandler creates a mock API handler for testing
func CreateMockAPIHandler(t *testing.T, objects []runtime.Object) api.Handler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2beta1pb.AddToScheme(scheme))

	k8sClient := fake.NewFakeClientWithScheme(scheme, objects...)
	return apiHandler.NewFakeAPIHandler(k8sClient)
}

// CreateDefaultTestProject creates a default test project for testing
func CreateDefaultTestProject() *v2beta1pb.Project {
	return &v2beta1pb.Project{
		ObjectMeta: v1.ObjectMeta{
			Name:      DefaultTestNamespace,
			Namespace: DefaultTestNamespace,
		},
		Spec: v2beta1pb.ProjectSpec{
			Tier: DefaultTestTier,
			Owner: &v2beta1pb.OwnerInfo{
				OwningTeam: DefaultTestTeam,
			},
		},
	}
}

// CreateMockMTLSHandler creates a mock MTLS handler for testing
func CreateMockMTLSHandler(t *testing.T,
	mockFliprClient *fliprmock.MockFliprClient,
	mockFliprConstraintsBuilder *typesmock.MockFliprConstraintsBuilder,
	enableMTLS bool,
	enableRuntimeClass bool,
	mtlsError error,
	runtimeClassError error,
	objects ...runtime.Object) types.MTLSHandler {

	if len(objects) == 0 {
		objects = []runtime.Object{CreateDefaultTestProject()}
	}

	testProject := objects[0].(*v2beta1pb.Project)
	apiHandler := CreateMockAPIHandler(t, objects)
	mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(map[string]interface{}{
		"project_name": testProject.Name,
		"tier":         testProject.Spec.Tier,
	}).AnyTimes()

	// Set up expectation for EnableMTLS
	if mtlsError != nil {
		mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
			Return(false, mtlsError).AnyTimes()
	} else {
		mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
			Return(enableMTLS, nil).AnyTimes()
	}

	// Set up expectation for EnableMTLSRuntimeClass
	if runtimeClassError != nil {
		mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
			Return(false, runtimeClassError).AnyTimes()
	} else {
		mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
			Return(enableRuntimeClass, nil).AnyTimes()
	}

	return utils.NewMTLSHandler(apiHandler, mockFliprClient, mockFliprConstraintsBuilder)
}
