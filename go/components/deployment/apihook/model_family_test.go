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

	"github.com/michelangelo-ai/michelangelo/go/api/apimocks"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func TestFulfillModelFamily_PopulatesFromModelOnCreate(t *testing.T) {
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

func TestFulfillModelFamily_KeepsMatchingRequestedFamily(t *testing.T) {
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

func TestFulfillModelFamily_RejectsMismatchedRequestedFamily(t *testing.T) {
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

func TestFulfillModelFamily_NoDesiredRevisionIsNoop(t *testing.T) {
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

func TestFulfillModelFamily_ModelWithoutFamilyLeavesFamilyUnset(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetModel(mockHandler, "")

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.CreateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeCreate(context.Background(), request)

	require.NoError(t, err)
	assert.Nil(t, request.Deployment.Spec.ModelFamily)
}

func TestFulfillModelFamily_ModelNotFoundIsNotAnError(t *testing.T) {
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

func TestFulfillModelFamily_ModelGetErrorIsReturned(t *testing.T) {
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

func TestFulfillModelFamily_DefaultsModelNamespaceFromDeployment(t *testing.T) {
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

func TestFulfillModelFamily_BackfillsFamilyOnLegacyDeploymentUpdate(t *testing.T) {
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

func TestFulfillModelFamily_AllowsNewModelInSameFamily(t *testing.T) {
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

func TestFulfillModelFamily_RejectsModelFromDifferentFamily(t *testing.T) {
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

func TestFulfillModelFamily_RetirementSkipsModelLookup(t *testing.T) {
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
