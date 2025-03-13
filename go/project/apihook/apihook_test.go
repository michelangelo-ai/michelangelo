package apihook

import (
	"context"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	k8sCoreClient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"testing"
)

func TestCreateProject(t *testing.T) {
	request := &v2.CreateProjectRequest{
		Project: &v2.Project{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test",
				Namespace: "test",
			},
			Spec: v2.ProjectSpec{
				Tier:        2,
				Description: "unit test project",
				RootDir:     "/test-dir",
				Owner: &v2.OwnerInfo{
					OwningTeam: "1234",
				},
			},
		},
	}

	testCases := []struct {
		name          string
		expectedError bool
		req           *v2.CreateProjectRequest
		setup         func(client k8sCoreClient.CoreV1Interface, clientset *fakek8sclient.Clientset)
		assert        func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface)
	}{
		{
			name:  "create new namespace",
			req:   request,
			setup: nil,
			assert: func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface) {
				assert.NoError(t, err)
				list, err := client.Namespaces().List(context.Background(), v1.ListOptions{})
				assert.NoError(t, err)
				assert.Len(t, list.Items, 1)
				assert.Equal(t, "test", list.Items[0].Name)
			},
		},
		{
			name: "create a project with existing namespace",
			req:  request,
			setup: func(client k8sCoreClient.CoreV1Interface, _ *fakek8sclient.Clientset) {
				namespace := corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "namespace",
						APIVersion: "core/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: request.Project.Namespace,
					},
					Spec: corev1.NamespaceSpec{},
				}
				_, err := client.Namespaces().Create(context.Background(), &namespace, v1.CreateOptions{})
				assert.NoError(t, err)
			},
			assert: func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface) {
				assert.NoError(t, err)
				list, err := client.Namespaces().List(context.Background(), v1.ListOptions{})
				assert.NoError(t, err)
				assert.Len(t, list.Items, 1)
				assert.Equal(t, "test", list.Items[0].Name)
			},
		},
		{
			name: "failed to create namespace",
			req:  request,
			setup: func(_ k8sCoreClient.CoreV1Interface, clientset *fakek8sclient.Clientset) {
				clientset.PrependReactor("create", "namespaces", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, assert.AnError
				})
			},
			assert: func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface) {
				assert.Error(t, err, "failed to create namespace: assert.AnError general error for testing")
			},
		},
		{
			name:          "create project with a name different from namespace throws error",
			expectedError: true,
			req: &v2.CreateProjectRequest{
				Project: &v2.Project{
					ObjectMeta: v1.ObjectMeta{
						Name:      "test_project",
						Namespace: "test",
					},
					Spec: v2.ProjectSpec{
						Tier:        2,
						Description: "unit test project",
						RootDir:     "/test-dir",
						Owner: &v2.OwnerInfo{
							OwningTeam: "1234",
						},
					},
				},
			},
			setup: nil,
			assert: func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface) {
				assert.Error(t, err, "project name and namespace cannot be different")
			},
		},
		{
			name:          "create project with name = 'default' throws error",
			expectedError: true,
			req: &v2.CreateProjectRequest{
				Project: &v2.Project{
					ObjectMeta: v1.ObjectMeta{
						Name:      "default",
						Namespace: "default",
					},
					Spec: v2.ProjectSpec{
						Tier:        2,
						Description: "unit test project",
						RootDir:     "/test-dir",
						Owner: &v2.OwnerInfo{
							OwningTeam: "1234",
						},
					},
				},
			},
			setup: nil,
			assert: func(t *testing.T, err error, client k8sCoreClient.CoreV1Interface) {
				assert.Error(t, err, "users are forbidden to create project in default or system namespace")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientSet := fakek8sclient.NewClientset()
			client := clientSet.CoreV1()
			fakeAPIHook := apiHook{
				logger:     zap.NewNop(),
				apiHandler: nil,
				k8sClient:  client,
			}

			if tc.setup != nil {
				tc.setup(client, clientSet)
			}
			err := fakeAPIHook.BeforeCreate(context.Background(), tc.req)
			tc.assert(t, err, client)
		})
	}
}

func TestGetProjectAPIHook(t *testing.T) {
	logger := zap.NewNop()
	k8sRestConfig := &rest.Config{}
	result, err := GetProjectAPIHook(logger, nil, k8sRestConfig)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.APIHook)
}
