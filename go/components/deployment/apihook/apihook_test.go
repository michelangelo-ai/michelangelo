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

// Shared fixtures for the hook's tests. Each concern's own behaviour is tested
// alongside its source file (e.g. model_family_test.go); this file covers the
// request plumbing in apihook.go.

const (
	testNamespace  = "test-namespace"
	testDeployment = "my-deployment"
	testModel      = "bert-cola-38"
	testFamily     = "bert-cola"
)

func newDeployment(modelFamily *apipb.ResourceIdentifier) *v2.Deployment {
	return &v2.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: testDeployment},
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
		Get(gomock.Any(), testNamespace, testDeployment, gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		DoAndReturn(func(_ context.Context, _, _ string, _ *metav1.GetOptions, obj interface{}) error {
			existing := obj.(*v2.Deployment)
			if familyName != "" {
				existing.Spec.ModelFamily = &apipb.ResourceIdentifier{Namespace: testNamespace, Name: familyName}
			}
			return nil
		})
}

func TestBeforeCreate_NilDeploymentIsNoop(t *testing.T) {
	hook := apiHook{logger: zap.NewNop()}

	err := hook.BeforeCreate(context.Background(), &v2.CreateDeploymentRequest{})

	assert.NoError(t, err)
}

func TestBeforeUpdate_NilDeploymentIsNoop(t *testing.T) {
	hook := apiHook{logger: zap.NewNop()}

	err := hook.BeforeUpdate(context.Background(), &v2.UpdateDeploymentRequest{})

	assert.NoError(t, err)
}

func TestBeforeUpdate_PassesExistingDeploymentToSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	expectGetDeployment(mockHandler, testFamily)
	expectGetModel(mockHandler, "other-family")

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeUpdate(context.Background(), request)

	// Only a step that saw the stored Deployment can detect the family change.
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestBeforeUpdate_ExistingDeploymentNotFoundFallsBackToCreateBehaviour(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockHandler := apimocks.NewMockHandler(ctrl)
	mockHandler.EXPECT().
		Get(gomock.Any(), testNamespace, testDeployment, gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		Return(apiErrors.NewNotFound(schema.GroupResource{Resource: "deployments"}, testDeployment))
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
		Get(gomock.Any(), testNamespace, testDeployment, gomock.Any(), gomock.AssignableToTypeOf(&v2.Deployment{})).
		Return(assert.AnError)

	hook := apiHook{logger: zap.NewNop(), apiHandler: mockHandler}
	request := &v2.UpdateDeploymentRequest{Deployment: newDeployment(nil)}

	err := hook.BeforeUpdate(context.Background(), request)

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRegisterDeploymentAPIHook(t *testing.T) {
	v2.ClearDeploymentAPIHooks()
	t.Cleanup(v2.ClearDeploymentAPIHooks)

	RegisterDeploymentAPIHook(zap.NewNop(), nil)

	assert.Len(t, v2.GetDeploymentAPIHooks(), 1)
}
