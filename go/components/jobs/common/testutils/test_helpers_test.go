package testutils

import (
	"fmt"
	"testing"

	"code.uber.internal/base/testing/contextmatcher"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	v2beta1pb "michelangelo/api/v2beta1"
	"mock/code.uber.internal/rt/flipr-client-go.git/flipr/fliprmock"
	"mock/code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types/typesmock"
)

func TestCreateDefaultTestProject(t *testing.T) {
	project := CreateDefaultTestProject()

	assert.Equal(t, DefaultTestNamespace, project.Name)
	assert.Equal(t, DefaultTestNamespace, project.Namespace)
	assert.Equal(t, int32(DefaultTestTier), project.Spec.Tier)
	assert.Equal(t, DefaultTestTeam, project.Spec.Owner.OwningTeam)
}

func TestCreateMockAPIHandler(t *testing.T) {
	project := CreateDefaultTestProject()
	objects := []runtime.Object{project}

	apiHandler := CreateMockAPIHandler(t, objects)
	assert.NotNil(t, apiHandler, "API handler should not be nil")
}

func TestCreateMockMTLSHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFliprClient := fliprmock.NewMockFliprClient(ctrl)
	mockFliprConstraintsBuilder := typesmock.NewMockFliprConstraintsBuilder(ctrl)

	testCases := []struct {
		name               string
		enableMTLS         bool
		enableRuntimeClass bool
		mtlsError          error
		runtimeClassError  error
	}{
		{
			name:               "MTLS enabled, Runtime Class enabled",
			enableMTLS:         true,
			enableRuntimeClass: true,
			mtlsError:          nil,
			runtimeClassError:  nil,
		},
		{
			name:               "MTLS enabled, Runtime Class disabled",
			enableMTLS:         true,
			enableRuntimeClass: false,
			mtlsError:          nil,
			runtimeClassError:  nil,
		},
		{
			name:               "MTLS disabled, Runtime Class enabled",
			enableMTLS:         false,
			enableRuntimeClass: true,
			mtlsError:          nil,
			runtimeClassError:  nil,
		},
		{
			name:               "MTLS disabled, Runtime Class disabled",
			enableMTLS:         false,
			enableRuntimeClass: false,
			mtlsError:          nil,
			runtimeClassError:  nil,
		},
		{
			name:               "MTLS error",
			enableMTLS:         false,
			enableRuntimeClass: false,
			mtlsError:          fmt.Errorf("MTLS error"),
			runtimeClassError:  nil,
		},
		{
			name:               "Runtime Class error",
			enableMTLS:         false,
			enableRuntimeClass: false,
			mtlsError:          nil,
			runtimeClassError:  fmt.Errorf("Runtime Class error"),
		},
		{
			name:               "Both errors",
			enableMTLS:         false,
			enableRuntimeClass: false,
			mtlsError:          fmt.Errorf("MTLS error"),
			runtimeClassError:  fmt.Errorf("Runtime Class error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test with default project
			mtlsHandler := CreateMockMTLSHandler(t, mockFliprClient, mockFliprConstraintsBuilder, tc.enableMTLS, tc.enableRuntimeClass, tc.mtlsError, tc.runtimeClassError)
			assert.NotNil(t, mtlsHandler, "MTLS handler should not be nil")

			// Test with custom project
			customProject := &v2beta1pb.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-project",
					Namespace: "custom-namespace",
				},
				Spec: v2beta1pb.ProjectSpec{
					Tier: 1,
					Owner: &v2beta1pb.OwnerInfo{
						OwningTeam: "custom-team",
					},
				},
			}

			mockFliprConstraintsBuilder.EXPECT().GetFliprConstraints(map[string]interface{}{
				"project_name": customProject.Name,
				"tier":         customProject.Spec.Tier,
			}).AnyTimes()

			// Set up MTLS expectation based on whether there's an error
			if tc.mtlsError != nil {
				mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
					Return(false, tc.mtlsError).AnyTimes()
			} else {
				mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
					Return(tc.enableMTLS, nil).AnyTimes()
			}

			// Set up Runtime Class expectation based on whether there's an error
			if tc.runtimeClassError != nil {
				mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
					Return(false, tc.runtimeClassError).AnyTimes()
			} else {
				mockFliprClient.EXPECT().GetBoolValue(contextmatcher.Any(), "enableMTLS", gomock.Any(), false).
					Return(tc.enableRuntimeClass, nil).AnyTimes()
			}

			mtlsHandlerWithCustomProject := CreateMockMTLSHandler(t, mockFliprClient, mockFliprConstraintsBuilder, tc.enableMTLS, tc.enableRuntimeClass, tc.mtlsError, tc.runtimeClassError, customProject)
			assert.NotNil(t, mtlsHandlerWithCustomProject, "MTLS handler with custom project should not be nil")
		})
	}
}
