package apihook

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	testNamespace = "test-namespace"
	testModel     = "bert-cola-38"
	testFamily    = "bert-cola"
)

func newDeployment(modelFamily *apipb.ResourceIdentifier) *v2.Deployment {
	return &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "my-deployment"},
		Spec: v2.DeploymentSpec{
			DesiredRevision: &apipb.ResourceIdentifier{Namespace: testNamespace, Name: testModel},
			ModelFamily:     modelFamily,
		},
	}
}

// expectGetModel stubs the Model lookup to return a Model whose family is familyName
// (or no family when familyName is empty).
func expectGetModel(mockHandler *apimocks.MockHandler, familyName string) {
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, testModel, gomock.Any(), gomock.AssignableToTypeOf(&v2.Model{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			model := obj.(*v2.Model)
			model.ObjectMeta = metav1.ObjectMeta{Namespace: testNamespace, Name: testModel}
			if familyName != "" {
				model.Spec.ModelFamily = &apipb.ResourceIdentifier{Namespace: testNamespace, Name: familyName}
			}
			return nil
		})
}

// expectGetDeployment stubs the existing-Deployment lookup done by BeforeUpdate.
func expectGetDeployment(mockHandler *apimocks.MockHandler, familyName string) {
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, "my-deployment", gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			existing := obj.(*v2.Deployment)
			if familyName != "" {
				existing.Spec.ModelFamily = &apipb.ResourceIdentifier{Namespace: testNamespace, Name: familyName}
			}
			return nil
		})
}

func TestBeforeCreate_PopulatesModelFamilyFromModel(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, &apipb.ResourceIdentifier{Namespace: testNamespace, Name: testFamily},
		request.Deployment.Spec.ModelFamily)
}

func TestBeforeCreate_KeepsMatchingRequestedFamily(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{
		Deployment: newDeployment(&apipb.ResourceIdentifier{Name: testFamily}),
	}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
	// The namespace is filled in from the Model's family reference.
	assert.Equal(t, testNamespace, request.Deployment.Spec.ModelFamily.GetNamespace())
}

func TestBeforeCreate_RejectsMismatchedRequestedFamily(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{
		Deployment: newDeployment(&apipb.ResourceIdentifier{Name: "other-family"}),
	}

	err := hook.BeforeCreate(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "does not match the model family")
}

func TestBeforeCreate_NoDesiredRevisionIsNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl) // no Get expected

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	deployment := newDeployment(nil)
	deployment.Spec.DesiredRevision = nil
	request := &v2.CreateDeploymentRequest{Deployment: deployment}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Nil(t, request.Deployment.Spec.ModelFamily)
}

func TestBeforeCreate_NilDeploymentIsNoop(t *testing.T) {
	hook := apiHook{logger: zap.NewNop()}

	err := hook.BeforeCreate(context.Background(), &v2.CreateDeploymentRequest{})

	assert.NoError(t, err)
}

func TestBeforeCreate_ModelWithoutFamilyLeavesFamilyUnset(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, "")

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Nil(t, request.Deployment.Spec.ModelFamily)
}

func TestBeforeCreate_ModelNotFoundIsNotAnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, testModel, gomock.Any(), gomock.Any()).
		Return(status.Error(codes.NotFound, "model not found"))

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeCreate(context.Background(), request)

	// The controller reports a missing model on the AssetsPrepared condition;
	// blocking the write here would hide that signal.
	require.NoError(t, err)
	assert.Nil(t, request.Deployment.Spec.ModelFamily)
}

func TestBeforeCreate_ModelGetErrorIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, testModel, gomock.Any(), gomock.Any()).
		Return(assert.AnError)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeCreate(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestBeforeCreate_DefaultsModelNamespaceFromDeployment(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	deployment := newDeployment(nil)
	deployment.Spec.DesiredRevision = &apipb.ResourceIdentifier{Name: testModel}
	request := &v2.CreateDeploymentRequest{Deployment: deployment}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
}

func TestBeforeUpdate_BackfillsFamilyOnLegacyDeployment(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetDeployment(mockHandler, "")
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeUpdate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
}

func TestBeforeUpdate_AllowsNewModelInSameFamily(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetDeployment(mockHandler, testFamily)
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{
		Deployment: newDeployment(&apipb.ResourceIdentifier{Namespace: testNamespace, Name: testFamily}),
	}

	err := hook.BeforeUpdate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
}

func TestBeforeUpdate_RejectsModelFromDifferentFamily(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetDeployment(mockHandler, testFamily)
	expectGetModel(mockHandler, "other-family")

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	// The client echoes back the stored family, as the Studio update form does.
	request := &v2.UpdateDeploymentRequest{
		Deployment: newDeployment(&apipb.ResourceIdentifier{Namespace: testNamespace, Name: testFamily}),
	}

	err := hook.BeforeUpdate(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "cannot be changed")
}

func TestBeforeUpdate_RetirementSkipsModelLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetDeployment(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	deployment := newDeployment(&apipb.ResourceIdentifier{Namespace: testNamespace, Name: testFamily})
	deployment.Spec.DesiredRevision = nil
	request := &v2.UpdateDeploymentRequest{Deployment: deployment}

	err := hook.BeforeUpdate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
}

func TestBeforeUpdate_ExistingDeploymentNotFoundFallsBackToCreateBehaviour(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, "my-deployment", gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		Return(apiErrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, "my-deployment"))
	expectGetModel(mockHandler, testFamily)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeUpdate(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, testFamily, request.Deployment.Spec.ModelFamily.GetName())
}

func TestBeforeUpdate_ExistingDeploymentGetErrorIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, "my-deployment", gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		Return(assert.AnError)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeUpdate(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestBeforeUpdate_NilDeploymentIsNoop(t *testing.T) {
	hook := apiHook{logger: zap.NewNop()}

	err := hook.BeforeUpdate(context.Background(), &v2.UpdateDeploymentRequest{})

	assert.NoError(t, err)
}

func TestRegisterDeploymentAPIHook(t *testing.T) {
	v2.ClearDeploymentAPIHooks()
	t.Cleanup(v2.ClearDeploymentAPIHooks)

	RegisterDeploymentAPIHook(zap.NewNop(), nil)

	assert.Len(t, v2.GetDeploymentAPIHooks(), 1)
}
