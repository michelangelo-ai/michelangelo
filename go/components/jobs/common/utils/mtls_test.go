package utils

import (
	"testing"

	"code.uber.internal/base/testing/contextmatcher"
	"github.com/michelangelo-ai/michelangelo/go/api"
	apiHandler "github.com/michelangelo-ai/michelangelo/go/api/handler"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createTestAPIHandler(t *testing.T, objects []runtime.Object) api.Handler {
	scheme := runtime.NewScheme()
	require.NoError(t, v2beta1pb.AddToScheme(scheme))
	k8sClient := fake.NewFakeClientWithScheme(scheme, objects...)
	return apiHandler.NewFakeAPIHandler(k8sClient)
}

func createTestMTLSHandler(t *testing.T, mockFlipr *fliprmock.MockFliprClient, mockFliprConstraints *typesmock.MockFliprConstraintsBuilder, enable bool, project *v2beta1pb.Project) *MTLSHandlerImpl {
	apiHandler := createTestAPIHandler(t, []runtime.Object{project})

	mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
		"project_name": project.Name,
		"tier":         project.Spec.Tier,
	}).AnyTimes()

	mockFlipr.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
		Return(enable, nil).AnyTimes()

	handler := NewMTLSHandler(apiHandler, mockFlipr, mockFliprConstraints)
	mTLSHandler, ok := handler.(MTLSHandlerImpl)
	require.True(t, ok)
	return &mTLSHandler
}

func TestEnableMTLS(t *testing.T) {
	testCases := []struct {
		name           string
		projectName    string
		tier           int32
		expectedResult bool
		expectedError  error
	}{
		{
			name:           "mtls-testing project always returns true",
			projectName:    "mtls-testing",
			tier:           0,
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name:           "successful call returns flipr value",
			projectName:    "test-project",
			tier:           0,
			expectedResult: true,
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			project := &v2beta1pb.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.projectName,
					Namespace: tc.projectName,
				},
				Spec: v2beta1pb.ProjectSpec{
					Tier: tc.tier,
				},
			}

			mockFlipr := fliprmock.NewMockFliprClient(ctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(ctrl)
			mTLSHandler := createTestMTLSHandler(t, mockFlipr, mockFliprConstraints, tc.expectedResult, project)
			result, err := mTLSHandler.EnableMTLS(tc.projectName)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestNewMTLSHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPIHandler := apiHandler.NewFakeAPIHandler(nil)
	mockFlipr := fliprmock.NewMockFliprClient(ctrl)
	mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(ctrl)

	handler := NewMTLSHandler(mockAPIHandler, mockFlipr, mockFliprConstraints)
	require.NotNil(t, handler)

	mTLSHandler, ok := handler.(MTLSHandlerImpl)
	require.True(t, ok)

	require.Equal(t, mockAPIHandler, mTLSHandler.apiHandler)
	require.Equal(t, mockFlipr, mTLSHandler.fliprClient)
	require.Equal(t, mockFliprConstraints, mTLSHandler.fliprConstraintsBuilder)
}

func TestEnableMTLS_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	project := &v2beta1pb.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-error-project",
			Namespace: "non-error-project",
		},
		Spec: v2beta1pb.ProjectSpec{
			Tier: 0,
		},
	}

	mockAPIHandler := createTestAPIHandler(t, []runtime.Object{project})

	handler := MTLSHandlerImpl{
		apiHandler: mockAPIHandler,
	}

	result, err := handler.EnableMTLS("error-project")

	require.Error(t, err)
	require.True(t, utils.IsNotFoundError(err))
	require.False(t, result)
}

func TestEnableMTLSRuntimeClass(t *testing.T) {
	testCases := []struct {
		name           string
		projectName    string
		tier           int32
		expectedResult bool
		expectedError  error
	}{
		{
			name:           "mtls-runtime-class project always returns true",
			projectName:    "mtls-testing",
			tier:           0,
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name:           "successful call returns flipr value",
			projectName:    "test-project",
			tier:           0,
			expectedResult: true,
			expectedError:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			project := &v2beta1pb.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.projectName,
					Namespace: tc.projectName,
				},
				Spec: v2beta1pb.ProjectSpec{
					Tier: tc.tier,
				},
			}

			mockFlipr := fliprmock.NewMockFliprClient(ctrl)
			mockFliprConstraints := typesmock.NewMockFliprConstraintsBuilder(ctrl)
			apiHandler := createTestAPIHandler(t, []runtime.Object{project})

			mockFliprConstraints.EXPECT().GetFliprConstraints(map[string]interface{}{
				"project_name": project.Name,
				"tier":         project.Spec.Tier,
			}).AnyTimes()

			mockFlipr.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
				Return(tc.expectedResult, nil).AnyTimes()

			handler := NewMTLSHandler(apiHandler, mockFlipr, mockFliprConstraints)
			mTLSHandler, ok := handler.(MTLSHandlerImpl)
			require.True(t, ok)

			result, err := mTLSHandler.EnableMTLSRuntimeClass(tc.projectName)

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

func TestEnableMTLSRuntimeClass_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	project := &v2beta1pb.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-error-project",
			Namespace: "non-error-project",
		},
		Spec: v2beta1pb.ProjectSpec{
			Tier: 0,
		},
	}

	mockAPIHandler := createTestAPIHandler(t, []runtime.Object{project})

	handler := MTLSHandlerImpl{
		apiHandler: mockAPIHandler,
	}

	result, err := handler.EnableMTLSRuntimeClass("error-project")

	require.Error(t, err)
	require.True(t, utils.IsNotFoundError(err))
	require.False(t, result)
}
