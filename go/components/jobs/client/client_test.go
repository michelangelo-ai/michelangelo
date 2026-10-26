package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	gomock "github.com/golang/mock/gomock"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client/k8sengine"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	matypes "github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
)

// permissiveMapper returns a MockMapper with relaxed expectations on every
// method, matching the no-op behavior the original tests relied on from a
// real mapper. Tests that need specific mapper behavior should construct
// their own *typesmocks.MockMapper directly.
func permissiveMapper(t *testing.T) *typesmocks.MockMapper {
	ctrl := gomock.NewController(t)
	m := typesmocks.NewMockMapper(ctrl)
	m.EXPECT().GetLocalName(gomock.Any()).
		DoAndReturn(func(obj runtime.Object) (string, string) {
			if mo, ok := obj.(metav1.Object); ok {
				return "default", mo.GetName()
			}
			return "", ""
		}).AnyTimes()
	m.EXPECT().MapGlobalJobToLocal(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&rayv1.RayJob{}, nil).AnyTimes()
	m.EXPECT().MapGlobalJobClusterToLocal(gomock.Any(), gomock.Any()).
		Return(&rayv1.RayCluster{}, nil).AnyTimes()
	m.EXPECT().MapLocalClusterStatusToGlobal(gomock.Any()).
		Return(&matypes.JobClusterStatus{Ray: &v2pb.RayClusterStatus{}}, nil).AnyTimes()
	m.EXPECT().MapLocalJobStatusToGlobal(gomock.Any()).
		Return(&matypes.JobStatus{Ray: &v2pb.RayJobStatus{}}, nil).AnyTimes()
	return m
}

func TestGetClusterStatus(t *testing.T) {
	tests := []struct {
		msg                     string
		healthOk                bool
		sendErr                 error
		statusCode              int
		expectedConditionsTrue  []string
		expectedConditionsFalse []string
		expectErr               func(err error) bool
		getClientSetError       error
	}{
		{
			msg:                    "dummy error",
			healthOk:               true,
			sendErr:                nil,
			statusCode:             200,
			expectedConditionsTrue: []string{constants.ClusterReady},
			getClientSetError:      errors.New("dummy error"),
			expectErr:              func(err error) bool { return strings.Contains(err.Error(), "dummy error") },
		},
		{
			msg:                    "cluster online and healthy",
			healthOk:               true,
			sendErr:                nil,
			statusCode:             200,
			expectedConditionsTrue: []string{constants.ClusterReady},
		},
		{
			msg:                     "cluster online and unhealthy",
			healthOk:                false,
			sendErr:                 nil,
			statusCode:              200,
			expectedConditionsFalse: []string{constants.ClusterReady, constants.ClusterOffline},
		},
		{
			msg:                    "cluster offline",
			sendErr:                errors.New("connection refused"),
			expectedConditionsTrue: []string{constants.ClusterOffline},
			expectErr:              func(err error) bool { return strings.Contains(err.Error(), "connection refused") },
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				if test.sendErr != nil {
					return nil, test.sendErr
				}
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				var resp string
				if test.healthOk {
					resp = "ok"
				}
				return &http.Response{StatusCode: test.statusCode, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
			}

			// test
			status, err := k8sc.GetClusterStatus(context.Background(), &testCluster)
			if err == nil && test.expectErr != nil {
				t.Errorf("expected error, got nil for")
			}
			if err != nil {
				if test.expectErr == nil || !test.expectErr(err) {
					t.Errorf("unexpected error for %v", err)
				}
				return
			}

			// assert conditions
			require.Equal(t, len(status.StatusConditions), len(test.expectedConditionsFalse)+len(test.expectedConditionsTrue), "unexpected conditions")
			assertConditions(t, status.StatusConditions, test.expectedConditionsTrue, apipb.CONDITION_STATUS_TRUE)
			assertConditions(t, status.StatusConditions, test.expectedConditionsFalse, apipb.CONDITION_STATUS_FALSE)
		})
	}
}

func TestDeleteJob(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:               "dummy error from GetClientSetForCluster for a spark job",
			wantError:         "dummy error",
			jobInput:          &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "spark-job"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "resource name not empty condition for ray job",
			wantError: "resource name may not be empty",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				Ray:   c.RESTClient(),
				Spark: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper:  permissiveMapper(t),
			}

			// test
			err := k8sc.DeleteJob(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestDeletePromConfigMap(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "job1"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper:  permissiveMapper(t),
			}

			// test
			err := k8sc.DeletePromConfigMap(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestDeleteSecret(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			jobInput:          &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "unknown type used for body for ray job",
			wantError: "unknown type used for body:",
			jobInput:  &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
				header := http.Header{}
				header.Set("Content-Type", runtime.ContentTypeJSON)
				resp := "ok"
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
			})

			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			c.RESTClient().(*restclient.RESTClient).Client = fc

			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper:  permissiveMapper(t),
			}

			// test
			err := k8sc.DeleteSecret(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestWatcher(t *testing.T) {
	tests := []struct {
		msg               string
		watcherParams     []*WatcherParams
		want              []*ResourceWatcher
		wantError         string
		getClientSetError error
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg: "test empty watcher params",
		},
		{
			msg:           "test unknown watcher resource",
			wantError:     "unable to create watcher for unknown resource ",
			watcherParams: []*WatcherParams{{}},
		},
		{
			msg:           "test non empty watcher params",
			want:          []*ResourceWatcher{{}, {}, {}},
			watcherParams: []*WatcherParams{{ResourceName: constants.KubeRayResource}, {ResourceName: constants.KubeRayResource}, {ResourceName: "pods"}},
		},
		{
			msg:           "test kuberay job watcher params",
			want:          []*ResourceWatcher{{}},
			watcherParams: []*WatcherParams{{ResourceName: constants.KubeRayJobResource}},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{}, test.getClientSetError)

			p := Params{
				Factory: f,
				Helper:  NewHelper(),
				Logger:  zaptest.NewLogger(t),
			}

			k8sc1 := NewClient(p)

			// test
			res, err := k8sc1.Watcher(test.watcherParams, &testCluster)
			if test.wantError != "" {
				require.Nil(t, res)
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, len(test.want), len(res))
			}
		})
	}
}

// TestWatcher_KubeRayResourcesUseRayClient asserts that both KubeRay resources
// are listed and watched through the cluster's Ray client, the one that decodes
// with the kuberay scheme.
func TestWatcher_KubeRayResourcesUseRayClient(t *testing.T) {
	g := gomock.NewController(t)
	f := computemocks.NewMockFactory(g)

	rayClient, coreClient := &fake.RESTClient{}, &fake.RESTClient{}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "testCluster"}}
	f.EXPECT().GetClientSetForCluster(cluster).Return(&compute.ClientSet{Ray: rayClient, CoreV1: coreClient}, nil)

	k8sc := NewClient(Params{
		Factory: f,
		Helper:  NewHelper(),
		Logger:  zaptest.NewLogger(t),
	})

	params := []*WatcherParams{
		{ResourceName: constants.KubeRayResource},
		{ResourceName: constants.KubeRayJobResource},
	}
	res, err := k8sc.Watcher(params, cluster)
	require.NoError(t, err)
	require.Len(t, res, len(params))
	for _, p := range params {
		assert.Same(t, rayClient, p.Client, p.ResourceName)
	}
}

func TestCreatePromConfigMap(t *testing.T) {
	tt := []struct {
		msg            string
		job            runtime.Object
		setupMock      func(g *gomock.Controller) FederatedClient
		configFilePath string
		wantError      bool
	}{
		{
			msg: "error from GetClientSetForCluster",
			setupMock: func(g *gomock.Controller) FederatedClient {
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Mapper:  permissiveMapper(t),
					Helper:  NewHelper(),
				})
			},
			wantError: true,
		},
		{
			msg: "prom config file not found",
			job: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) FederatedClient {
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, nil)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Helper:  NewHelper(),
					Mapper:  permissiveMapper(t),
				})
			},
			wantError: true,
		},
		{
			msg: "error creating configmap - create error",
			job: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test",
				},
			},
			setupMock: func(g *gomock.Controller) FederatedClient {
				fc := fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
					header := http.Header{}
					header.Set("Content-Type", runtime.ContentTypeJSON)
					// Return error response for POST requests (which CreateResource uses)
					if request.Method == "POST" {
						resp := `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"configmaps is forbidden","reason":"Forbidden","code":403}`
						return &http.Response{StatusCode: 403, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
					}
					resp := "ok"
					return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(bytes.NewReader([]byte(resp)))}, nil
				})

				c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
				c.RESTClient().(*restclient.RESTClient).Client = fc
				f := computemocks.NewMockFactory(g)
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
					CoreV1: c.RESTClient(),
				}, nil)
				return NewClient(Params{
					Factory: f,
					Logger:  zaptest.NewLogger(t),
					Helper:  NewHelper(),
					Mapper:  permissiveMapper(t),
				})
			},
			// file path will be created as temp file in test body
			configFilePath: "prometheus.yml",
			wantError:      true,
		},
		// drop success case; encoding/codec not wired in test env
	}

	for _, test := range tt {
		t.Run(test.msg, func(t *testing.T) {
			g := gomock.NewController(t)
			c := test.setupMock(g)

			cluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}

			configPath := test.configFilePath
			if configPath != "" && test.msg != "prom config file not found" {
				tmp := t.TempDir()
				configPath = tmp + "/" + configPath
				_ = os.WriteFile(configPath, []byte("global:\n  scrape_interval: 15s\n"), 0o644)
			}
			err := c.CreatePromConfigMap(context.Background(), test.job, &cluster, configPath)
			if test.wantError {
				require.NotNil(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestCreateSecret(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg:               "dummy error from GetClientSetForCluster",
			wantError:         "dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "failure - unknown type used for body",
			wantError: "unknown type used for body:",
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test"},
				Spec: v2pb.RayJobSpec{
					User: &v2pb.UserInfo{
						Name: "test-user",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			// setup client
			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})

			g := gomock.NewController(t)
			f := computemocks.NewMockFactory(g)

			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
			}
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				CoreV1: c.RESTClient(),
			}, test.getClientSetError)

			provider := secrets.New(secrets.Params{}).SecretProvider

			k8sc := Client{
				factory:         f,
				helper:          NewHelper(),
				secretsProvider: provider,
				mapper:          permissiveMapper(t),
			}

			// test
			err := k8sc.CreateSecret(context.Background(), test.jobInput, &testCluster)
			if test.wantError != "" {
				require.NotNil(t, err)
				require.Error(t, err)
			} else {
				require.Nil(t, err)
			}
		})
	}
}

// getResourceHelper is a Helper whose GetResource fails with err.
type getResourceHelper struct {
	Helper
	err error
}

func (h getResourceHelper) GetResource(context.Context, restclient.Interface, string, string, string, runtime.Object) error {
	return h.err
}

// TestGetJobStatus tests the GetJobStatus method
func TestGetJobStatus(t *testing.T) {
	mkRayV2 := func(name string) *v2pb.RayJob { return &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: name}} }
	rayJobs := schema.GroupResource{Group: "ray.io", Resource: "rayjobs"}
	clientSet := func(f *computemocks.MockFactory) {
		f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
	}

	tests := []struct {
		name         string
		jobObject    runtime.Object
		setupFactory func(f *computemocks.MockFactory)
		getErr       error
		expectErr    string
		expectState  v2pb.RayJobState
		expectStatus string
		expectMsg    string
	}{
		{
			name:         "KubeRay RayJob not found",
			jobObject:    mkRayV2("job-a"),
			setupFactory: clientSet,
			getErr:       apierrors.NewNotFound(rayJobs, "job-a"),
			expectState:  v2pb.RAY_JOB_STATE_FAILED,
			expectStatus: "Unknown",
			expectMsg:    "Resource not found in compute cluster",
		},
		{
			// A proxy or ingress in front of the compute cluster answering 404
			// itself. client-go reports that as NotFound too, flagged as an
			// unexpected server response, but it says nothing about the RayJob.
			name:         "404 that did not come from the API server",
			jobObject:    mkRayV2("job-a"),
			setupFactory: clientSet,
			getErr:       apierrors.NewGenericServerResponse(http.StatusNotFound, "get", rayJobs, "job-a", "page not found", 0, true),
			expectErr:    "get ray job",
		},
		{
			name:         "GetResource error",
			jobObject:    mkRayV2("job-a"),
			setupFactory: clientSet,
			getErr:       apierrors.NewServiceUnavailable("compute cluster unavailable"),
			expectErr:    "get ray job",
		},
		{
			name:      "GetClientSetForCluster error",
			jobObject: mkRayV2("job-a"),
			setupFactory: func(f *computemocks.MockFactory) {
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
			},
			expectErr: "get client for cluster err",
		},
		{
			// Was "unsupported Spark job type" / "Spark job status not
			// implemented". The Spark arm is now real and takes the same shape
			// as Ray: it resolves a client set first, so a factory failure
			// surfaces identically. The Spark happy path and its NotFound
			// behaviour need assertions on JobStatus.Spark, which this
			// shared runner does not look at, so they live in
			// TestGetJobStatusSparkJob / TestGetJobStatusSparkJobNotFound.
			name:      "Spark job GetClientSetForCluster error",
			jobObject: &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Name: "spark-a"}},
			setupFactory: func(f *computemocks.MockFactory) {
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
			},
			expectErr: "get client for cluster err",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gctrl := gomock.NewController(t)
			defer gctrl.Finish()
			f := computemocks.NewMockFactory(gctrl)
			cl := &Client{factory: f, helper: NewHelper(), mapper: permissiveMapper(t), logger: zaptest.NewLogger(t)}
			if tt.getErr != nil {
				cl.helper = getResourceHelper{Helper: cl.helper, err: tt.getErr}
			}
			if tt.setupFactory != nil {
				tt.setupFactory(f)
			}
			cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
			js, err := cl.GetJobStatus(context.Background(), tt.jobObject, cluster)
			if tt.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectErr)
				assert.Nil(t, js, "a failed call must not report a status")
				return
			}
			require.NoError(t, err)
			var gotState v2pb.RayJobState
			var gotStatus, gotMsg string
			if js != nil && js.Ray != nil {
				gotState = js.Ray.State
				gotStatus = js.Ray.JobStatus
				gotMsg = js.Ray.Message
			}
			assert.Equal(t, tt.expectState, gotState)
			assert.Equal(t, tt.expectStatus, gotStatus)
			assert.Equal(t, tt.expectMsg, gotMsg)
		})
	}
}

func TestCreateJob(t *testing.T) {
	tests := []struct {
		msg               string
		jobInput          runtime.Object
		jobClusterInput   runtime.Object
		getClientSetError error
		wantError         string
	}{
		{
			msg: "dummy error from GetClientSetForCluster",
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec:   v2pb.RayJobSpec{},
				Status: v2pb.RayJobStatus{},
			},
			jobClusterInput: &v2pb.RayCluster{
				Spec: v2pb.RayClusterSpec{
					Head: &v2pb.RayHeadSpec{
						Pod: &v1.PodTemplateSpec{},
					},
					Workers: []*v2pb.RayWorkerSpec{
						{
							Pod: &v1.PodTemplateSpec{},
						},
					},
				},
			},
			wantError:         "get client for cluster err:dummy error",
			getClientSetError: errors.New("dummy error"),
		},
		{
			msg:       "ray job create failure - remote cluster GET fails with empty name",
			wantError: "get remote ray cluster for owner ref: resource name may not be empty",
			jobInput: &v2pb.RayJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
				},
				Spec:   v2pb.RayJobSpec{},
				Status: v2pb.RayJobStatus{},
			},
			jobClusterInput: &v2pb.RayCluster{},
		},
	}

	for _, test := range tests {
		t.Run(test.msg, func(t *testing.T) {
			testCluster := v2pb.Cluster{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Cluster",
					APIVersion: "michelangelo.uber.com/v2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "testCluster",
				},
				Spec: v2pb.ClusterSpec{
					Cluster: &v2pb.ClusterSpec_Kubernetes{
						Kubernetes: &v2pb.KubernetesSpec{
							Rest: &v2pb.ConnectionSpec{
								Host: "test",
							},
						},
					},
				},
			}

			g := gomock.NewController(t)

			// setup client
			c := discovery.NewDiscoveryClientForConfigOrDie(&restclient.Config{})
			f := computemocks.NewMockFactory(g)
			f.EXPECT().GetClientSetForCluster(&testCluster).Return(&compute.ClientSet{
				Ray: c.RESTClient(),
			}, test.getClientSetError)

			k8sc := Client{
				factory: f,
				helper:  NewHelper(),
				mapper:  permissiveMapper(t),
			}

			// test
			err := k8sc.CreateJob(context.Background(), test.jobInput, test.jobClusterInput, &testCluster)

			if test.wantError != "" {
				require.NotNil(t, err)
				require.Equal(t, test.wantError, err.Error())
			} else {
				require.Nil(t, err)
			}
		})
	}
}

func TestGetJobClusterStatus_ClientSetError(t *testing.T) {
	g := gomock.NewController(t)
	f := computemocks.NewMockFactory(g)
	cl := &Client{factory: f, helper: NewHelper(), mapper: permissiveMapper(t)}
	jobCluster := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc1"}}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}

	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, assert.AnError)
	_, err := cl.GetJobClusterStatus(context.Background(), jobCluster, cluster)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get client for cluster err")
}

func assertConditions(
	t *testing.T,
	conditions []*apipb.Condition,
	expectedConditionTypes []string,
	conditionStatus apipb.ConditionStatus,
) {
	wantCount := len(expectedConditionTypes)
	haveCount := 0
	for _, wantCond := range expectedConditionTypes {
		haveConditions := conditions
		for _, haveCond := range haveConditions {
			if wantCond == haveCond.Type {
				haveCount++
				require.Equal(t, conditionStatus, haveCond.Status)
			}
		}
	}
	require.Equal(t, wantCount, haveCount, "not all conditions were present")
}

// TestCreateJobSparkJob asserts that CreateJob projects a *v2pb.SparkJob into
// the member cluster: it POSTs the mapped SparkApplication through
// ClientSet.Spark, to constants.KubeSparkResource in the mapper's local
// namespace. (This case previously asserted the "not supported" error the arm
// used to return.)
//
// jobClusterObject is deliberately nil. A SparkApplication has no parent
// CR in the member cluster, so the Ray arm's owner-reference lookup must not be
// mirrored here. If it were, GetLocalName(nil) would come back empty and this
// call would fail with "resource name may not be empty" instead of succeeding.
func TestCreateJobSparkJob(t *testing.T) {
	g := gomock.NewController(t)
	defer g.Finish()

	rec := &sparkRequestRecord{}
	f := computemocks.NewMockFactory(g)
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:   forbiddenRESTClient(t),
		Spark: sparkRESTClient(rec, http.StatusOK, sparkApplicationJSON("")),
	}, nil)

	cl := &Client{
		factory: f,
		helper:  NewHelper(),
		mapper:  sparkMapper(t, "test-spark-job"),
		logger:  zaptest.NewLogger(t),
	}

	sparkJob := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-spark-job",
		},
	}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	err := cl.CreateJob(context.Background(), sparkJob, nil, cluster)
	require.NoError(t, err)

	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.True(t, strings.HasSuffix(rec.path, "/namespaces/default/"+constants.KubeSparkResource),
		"create should POST to the mapper's local namespace and the sparkapplications resource, got %q", rec.path)
}

// TestDeleteJobSparkJob asserts that DeleteJob actually issues the DELETE for a
// projected SparkApplication through ClientSet.Spark. Unlike a RayJob, a
// SparkApplication has no owning CR in the member cluster to cascade-GC it,
// so a no-op here would leak a running Spark driver.
//
// The DELETE is preceded by a GET (the application under the job's name
// may not be this job's), so the happy path is two requests, the delete last.
// The ownership rules themselves are TestDeleteJobSparkJobOwnership's; here the
// application carries the job's UID, which is what makes it the job's own.
func TestDeleteJobSparkJob(t *testing.T) {
	rec := &sparkRequestRecord{}
	cl, _ := newSparkTestClient(t,
		scriptedSparkRESTClient(t, rec,
			sparkAppReply(t, testAppUID, map[string]string{constants.JobUIDLabelKey: testJobUID}),
			sparkDeletedReply),
		sparkMapper(t, "test-spark-job"))

	sparkJob := newSparkJobWithUID(testJobUID)
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	err := cl.DeleteJob(context.Background(), sparkJob, cluster)
	require.NoError(t, err)

	assert.Equal(t, 2, rec.calls, "one GET to read the application, one DELETE")
	assert.Equal(t, http.MethodDelete, rec.method, "the DELETE comes last")
	assert.True(t, strings.HasSuffix(rec.path, "/namespaces/default/"+constants.KubeSparkResource+"/test-spark-job"),
		"delete should target the named sparkapplications resource, got %q", rec.path)
}

// TestGetJobStatusSparkJob asserts the happy path: GetJobStatus reads the
// SparkApplication through ClientSet.Spark and returns a *matypes.JobStatus
// whose Spark side is populated. sparkMapper type-asserts that what reached
// MapLocalJobStatusToGlobal really was a decoded *sparkv1beta2.SparkApplication.
// The application carries the job's UID: one that did not would not be the
// job's own (TestGetJobStatusSparkJobOwnership).
func TestGetJobStatusSparkJob(t *testing.T) {
	g := gomock.NewController(t)
	defer g.Finish()

	rec := &sparkRequestRecord{}
	own := sparkAppReply(t, testAppUID, map[string]string{constants.JobUIDLabelKey: testJobUID})
	f := computemocks.NewMockFactory(g)
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:   forbiddenRESTClient(t),
		Spark: sparkRESTClient(rec, own.status, own.body),
	}, nil)

	cl := &Client{
		factory: f,
		helper:  NewHelper(),
		mapper:  sparkMapper(t, "test-spark-job"),
		logger:  zaptest.NewLogger(t),
	}

	sparkJob := newSparkJobWithUID(testJobUID)
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	js, err := cl.GetJobStatus(context.Background(), sparkJob, cluster)
	require.NoError(t, err)
	require.NotNil(t, js)
	require.NotNil(t, js.Spark)
	assert.Nil(t, js.Ray)
	assert.Equal(t, "spark-app-1234", js.Spark.ApplicationId)

	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, http.MethodGet, rec.method)
	assert.True(t, strings.HasSuffix(rec.path, "/namespaces/default/"+constants.KubeSparkResource+"/test-spark-job"),
		"get should target the named sparkapplications resource, got %q", rec.path)
}

// TestGetJobStatusSparkJobNotFound: a missing SparkApplication is
// surfaced as an error, not turned into a synthetic terminal status the way the
// Ray arm does: what absence means is the caller's to decide (the SparkJob
// controller records reason SparkApplicationDeleted). The error is wrapped with
// %w, so apierrors.IsNotFound must still recognise it.
func TestGetJobStatusSparkJobNotFound(t *testing.T) {
	g := gomock.NewController(t)
	defer g.Finish()

	rec := &sparkRequestRecord{}
	f := computemocks.NewMockFactory(g)
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:   forbiddenRESTClient(t),
		Spark: sparkRESTClient(rec, http.StatusNotFound, sparkNotFoundStatusJSON),
	}, nil)

	cl := &Client{
		factory: f,
		helper:  NewHelper(),
		mapper:  sparkMapper(t, "test-spark-job"),
		logger:  zaptest.NewLogger(t),
	}

	sparkJob := &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace", Name: "test-spark-job"}}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	js, err := cl.GetJobStatus(context.Background(), sparkJob, cluster)
	require.Error(t, err)
	assert.Nil(t, js, "no synthetic status may be fabricated for a missing SparkApplication")
	assert.True(t, apierrors.IsNotFound(err),
		"NotFound must stay recoverable through the error wrap, got: %v", err)
	assert.Equal(t, 1, rec.calls)
}

// TestCreateJobSparkJobIssuesNoParentLookup pins the *absence* of the Ray
// arm's owner-reference block from CreateJob's Spark arm.
//
// TestCreateJobSparkJob above passes a nil jobClusterObject, so re-adding that
// block behind an `if jobClusterObject != nil` guard would leave it green.
// This case hands the Spark arm a non-nil parent and still requires that no
// parent-CR lookup goes out. Three independent tripwires:
//
//   - the mapper mock expects GetLocalName only for the SparkJob, so the
//     block's first statement -- GetLocalName(jobClusterObject) -- is an
//     unexpected call and gomock fails the test;
//   - ClientSet.Ray rejects every request, so the block's GetResource fails;
//   - ClientSet.Spark must see exactly one request, so a parent read routed
//     through the Spark client shows up as a second call.
//
// There is no parent CR to own a SparkApplication in the member cluster, so
// such a block would fail in production on the real nil jobClusterObject, with
// "get remote ray cluster for owner ref: resource name may not be empty"
// (GetLocalName(nil) is empty), and, guarded, would silently do nothing while
// looking deliberate.
func TestCreateJobSparkJobIssuesNoParentLookup(t *testing.T) {
	g := gomock.NewController(t)
	defer g.Finish()

	sparkJob := &v2pb.SparkJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace", Name: "test-spark-job"},
	}
	// A parent that exists only to be ignored.
	parent := &v2pb.RayCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "test-namespace", Name: "rc1"},
	}

	m := typesmocks.NewMockMapper(g)
	m.EXPECT().GetLocalName(sparkJob).Return("default", "test-spark-job").AnyTimes()
	m.EXPECT().MapGlobalJobToLocal(sparkJob, parent, gomock.Any()).
		Return(&sparkv1beta2.SparkApplication{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-spark-job"},
		}, nil)

	rec := &sparkRequestRecord{}
	f := computemocks.NewMockFactory(g)
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:   forbiddenRESTClient(t),
		Spark: sparkRESTClient(rec, http.StatusOK, sparkApplicationJSON("")),
	}, nil)

	cl := &Client{factory: f, helper: NewHelper(), mapper: m, logger: zaptest.NewLogger(t)}
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	require.NoError(t, cl.CreateJob(context.Background(), sparkJob, parent, cluster))

	assert.Equal(t, 1, rec.calls, "the Spark arm issues exactly one request: the create")
	assert.Equal(t, http.MethodPost, rec.method)
}

// sparkRequestRecord captures what a fake Spark REST client was actually asked
// to do, so a test can prove the call went out over ClientSet.Spark and hit the
// sparkapplications resource.
type sparkRequestRecord struct {
	calls  int
	method string // of the most recent request
	path   string // of the most recent request

	// requests is every request, in the order it arrived.
	requests []sparkCall
}

// sparkCall is one request a fake Spark REST client received.
type sparkCall struct {
	method string
	path   string
	query  string
	body   string
}

func (r *sparkRequestRecord) record(req *http.Request) {
	call := sparkCall{method: req.Method, path: req.URL.Path, query: req.URL.RawQuery}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			body = []byte("unreadable request body: " + err.Error())
		}
		call.body = string(body)
	}
	r.requests = append(r.requests, call)
	r.calls++
	r.method = call.method
	r.path = call.path
}

// lines renders the requests as "METHOD /path", the form the ownership tests
// compare whole request sequences in.
func (r *sparkRequestRecord) lines() []string {
	lines := make([]string, 0, len(r.requests))
	for _, call := range r.requests {
		lines = append(lines, call.method+" "+call.path)
	}
	return lines
}

// sparkRESTClient builds a recording fake REST client that replies with the
// given status and body.
//
// This deliberately does not use the discovery.NewDiscoveryClientForConfigOrDie
// idiom the Ray cases above use: discovery installs a runtime.NoopEncoder, so
// rest.Request.Body(runtime.Object) cannot encode a request body under it —
// that is what the "encoding/codec not wired in test env" note in
// TestCreatePromConfigMap is about. CreateJob's Spark arm POSTs a body, so it
// needs a real serializer. The wiring below matches the one already used in
// go/components/jobs/scheduler/kueue/localqueues_test.go, and stays in the same
// k8s.io/client-go/rest/fake package this file already imports.
func sparkRESTClient(rec *sparkRequestRecord, status int, body string) *fake.RESTClient {
	return &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			rec.record(req)
			return jsonResponse(status, body), nil
		}),
	}
}

// sparkReply is one scripted response of scriptedSparkRESTClient.
type sparkReply struct {
	status int
	body   string
	// contentType is the reply's Content-Type; "" means JSON, which is what
	// the API server sends.
	contentType string
}

// scriptedSparkRESTClient answers the Nth request with the Nth reply and
// records every request. A request beyond the script fails the test: an extra
// call (a retry that should not happen, a delete after a refused read) is
// exactly what these tests are written to catch.
func scriptedSparkRESTClient(t *testing.T, rec *sparkRequestRecord, replies ...sparkReply) *fake.RESTClient {
	t.Helper()
	return &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			rec.record(req)
			if rec.calls > len(replies) {
				t.Errorf("unscripted request #%d: %s %s", rec.calls, req.Method, req.URL.Path)
				return nil, errors.New("no scripted reply left")
			}
			reply := replies[rec.calls-1]
			resp := jsonResponse(reply.status, reply.body)
			if reply.contentType != "" {
				resp.Header.Set("Content-Type", reply.contentType)
			}
			return resp, nil
		}),
	}
}

func jsonResponse(status int, body string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", runtime.ContentTypeJSON)
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// forbiddenRESTClient fails the test if anything is sent through it. Wiring one
// into ClientSet.Ray is what proves a Spark arm reached for cs.Spark.
func forbiddenRESTClient(t *testing.T) *fake.RESTClient {
	t.Helper()
	return &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			t.Errorf("Spark arm used ClientSet.Ray: %s %s", req.Method, req.URL.Path)
			return nil, errors.New("ClientSet.Ray must not be used by a Spark arm")
		}),
	}
}

// sparkMapper is the Spark-shaped counterpart of permissiveMapper, which
// returns Ray objects and is shared with the Ray cases. Each arm asserts the
// seam type it is handed, so the Spark client code cannot pass the wrong object
// through and still look green.
func sparkMapper(t *testing.T, localName string) *typesmocks.MockMapper {
	t.Helper()
	ctrl := gomock.NewController(t)
	m := typesmocks.NewMockMapper(ctrl)
	m.EXPECT().GetLocalName(gomock.Any()).
		DoAndReturn(func(obj runtime.Object) (string, string) {
			if _, ok := obj.(*v2pb.SparkJob); !ok {
				t.Errorf("GetLocalName: want *v2pb.SparkJob, got %T", obj)
			}
			return "default", localName
		}).AnyTimes()
	m.EXPECT().MapGlobalJobToLocal(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(jobObject runtime.Object, jobClusterObject runtime.Object, cluster *v2pb.Cluster) (runtime.Object, error) {
			if _, ok := jobObject.(*v2pb.SparkJob); !ok {
				t.Errorf("MapGlobalJobToLocal: want *v2pb.SparkJob, got %T", jobObject)
			}
			// Spark has no parent CR in the member cluster: the client
			// must hand jobClusterObject straight through, nil included.
			assert.Nil(t, jobClusterObject)
			return &sparkv1beta2.SparkApplication{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: localName},
			}, nil
		}).AnyTimes()
	m.EXPECT().MapLocalJobStatusToGlobal(gomock.Any()).
		DoAndReturn(func(obj runtime.Object) (*matypes.JobStatus, error) {
			app, ok := obj.(*sparkv1beta2.SparkApplication)
			if !ok {
				t.Errorf("MapLocalJobStatusToGlobal: want *sparkv1beta2.SparkApplication, got %T", obj)
				return nil, errors.New("unexpected local job object type")
			}
			return &matypes.JobStatus{
				Spark: &v2pb.SparkJobStatus{ApplicationId: app.Status.SparkApplicationID},
			}, nil
		}).AnyTimes()
	return m
}

// sparkApplicationJSON is a SparkApplication as the member cluster's API server
// would return it.
func sparkApplicationJSON(appID string) string {
	return `{"apiVersion":"sparkoperator.k8s.io/v1beta2","kind":"SparkApplication",` +
		`"metadata":{"namespace":"default","name":"test-spark-job"},` +
		`"status":{"sparkApplicationId":"` + appID + `","applicationState":{"state":"RUNNING"}}}`
}

// sparkNotFoundStatusJSON is the metav1.Status body a real API server returns
// alongside a 404, so the 404 decodes to a genuine apierrors NotFound.
const sparkNotFoundStatusJSON = `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure",` +
	`"message":"sparkapplications.sparkoperator.k8s.io \"test-spark-job\" not found",` +
	`"reason":"NotFound",` +
	`"details":{"name":"test-spark-job","group":"sparkoperator.k8s.io","kind":"sparkapplications"},` +
	`"code":404}`

const (
	testSparkJobName = "test-spark-job"

	// testJobUID is the UID of the global job the Spark tests act for. otherJobUID
	// is the UID of a different job whose SparkApplication shares the member
	// cluster's namespace and may share the name.
	testJobUID  = "job-uid-1"
	otherJobUID = "job-uid-2"

	// testAppUID is the UID of the SparkApplication object itself. It differs from
	// every job UID on purpose: the delete precondition is the object's UID, and a
	// fixture where the two coincided could not tell them apart.
	testAppUID = "app-uid-1"

	sparkCollectionPath = "/namespaces/default/" + constants.KubeSparkResource
	sparkObjectPath     = sparkCollectionPath + "/" + testSparkJobName
)

// sparkAppResource is the group-resource an API server's errors name.
var sparkAppResource = sparkv1beta2.Resource(constants.KubeSparkResource)

// sparkDeletedReply is what an API server answers a successful DELETE with.
var sparkDeletedReply = sparkReply{
	status: http.StatusOK,
	body:   `{"kind":"Status","apiVersion":"v1","metadata":{},"status":"Success"}`,
}

// Two 404s whose body is not a Kubernetes Status, as they arrive on the wire.
// client-go turns each into a NotFound that is also IsUnexpectedServerError;
// TestSparkNonStatusNotFoundReplies pins exactly what it builds.
var (
	// proxyNotFoundReply is the 404 page of a proxy or an ingress in front of
	// the member cluster's API server.
	proxyNotFoundReply = sparkReply{
		status:      http.StatusNotFound,
		contentType: "text/html",
		body:        "<html>404 page not found</html>",
	}
	// apiNotServedReply is what a kube-apiserver answers on a path that no API
	// it serves covers, such as sparkapplications without the Spark Operator's
	// CRD: http.NotFound's plain text (k8s.io/apiserver
	// pkg/util/notfoundhandler).
	apiNotServedReply = sparkReply{
		status:      http.StatusNotFound,
		contentType: "text/plain; charset=utf-8",
		body:        "404 page not found\n",
	}
)

// proxyConflictReply is the 409 page of a proxy in front of the member
// cluster's API server. For any verb but POST, client-go turns it into a
// Conflict that is also IsUnexpectedServerError.
var proxyConflictReply = sparkReply{
	status:      http.StatusConflict,
	contentType: "text/html",
	body:        "<html>409 Conflict</html>",
}

func newSparkJobWithUID(uid types.UID) *v2pb.SparkJob {
	return &v2pb.SparkJob{ObjectMeta: metav1.ObjectMeta{
		Namespace: "test-namespace",
		Name:      testSparkJobName,
		UID:       uid,
	}}
}

func jsonReply(t *testing.T, status int, obj any) sparkReply {
	t.Helper()
	body, err := json.Marshal(obj)
	require.NoError(t, err)
	return sparkReply{status: status, body: string(body)}
}

// sparkAppReply is the 200 reply carrying the test job's SparkApplication as the
// member cluster's API server returns it. appUID is the object's own UID and
// labels are its labels.
func sparkAppReply(t *testing.T, appUID types.UID, labels map[string]string) sparkReply {
	t.Helper()
	return jsonReply(t, http.StatusOK, &sparkv1beta2.SparkApplication{
		TypeMeta: metav1.TypeMeta{APIVersion: "sparkoperator.k8s.io/v1beta2", Kind: "SparkApplication"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      testSparkJobName,
			UID:       appUID,
			Labels:    labels,
		},
		Status: sparkv1beta2.SparkApplicationStatus{
			SparkApplicationID: "spark-app-1234",
			AppState:           sparkv1beta2.ApplicationState{State: sparkv1beta2.RunningState},
		},
	})
}

// statusReply is the reply an API server gives a request it rejects: the
// metav1.Status the error carries, under its HTTP code.
func statusReply(t *testing.T, apiErr *apierrors.StatusError) sparkReply {
	t.Helper()
	status := apiErr.ErrStatus
	status.TypeMeta = metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}
	return jsonReply(t, int(status.Code), &status)
}

// newSparkTestClient is a Client whose compute cluster has the given Spark
// client. ClientSet.Ray rejects every request, so a Spark arm that reaches for
// it fails the test. The Client logs to an observer so a test can read what was
// logged at Info.
func newSparkTestClient(
	t *testing.T, spark restclient.Interface, mapper matypes.Mapper,
) (*Client, *observer.ObservedLogs) {
	t.Helper()
	f := computemocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:   forbiddenRESTClient(t),
		Spark: spark,
	}, nil)
	core, logs := observer.New(zapcore.InfoLevel)
	return &Client{factory: f, helper: NewHelper(), mapper: mapper, logger: zap.New(core)}, logs
}

// namingOnlyMapper resolves names and nothing else: a call to any other mapper
// method is unexpected and fails the test, which is how a test shows that an
// arm stopped before it got that far.
func namingOnlyMapper(t *testing.T) *typesmocks.MockMapper {
	t.Helper()
	m := typesmocks.NewMockMapper(gomock.NewController(t))
	m.EXPECT().GetLocalName(gomock.Any()).Return("default", testSparkJobName).AnyTimes()
	return m
}

// requireInfoLog finds the one Info entry whose message contains snippet and
// returns its fields.
func requireInfoLog(t *testing.T, logs *observer.ObservedLogs, snippet string) map[string]any {
	t.Helper()
	entries := logs.FilterLevelExact(zapcore.InfoLevel).FilterMessageSnippet(snippet).All()
	require.Len(t, entries, 1, "want one Info log containing %q, got: %v", snippet, logs.All())
	return entries[0].ContextMap()
}

// TestCreateJobSparkJobNameConflict pins the ownership rule (the ma/job-uid
// label) on the create side. Applications are named after their job in a
// namespace every project on the member cluster shares, so a 409 can mean two
// things and the client has to tell which.
//
// The job's own application, left by an earlier attempt whose result was lost,
// is "already launched": the error must stay recognisable as AlreadyExists
// through the wrap, because the controller reads it that way. Only an
// application whose label is the job's own non-empty UID is the job's. Any
// other is the opposite -- another job's, or one without the label, which
// Michelangelo did not create for this job -- and an error the controller would
// read as "launched" would hand this job an outcome that is not its own. It
// wraps ErrSparkApplicationConflict instead, which must not be AlreadyExists,
// and the message says whose the application is.
func TestCreateJobSparkJobNameConflict(t *testing.T) {
	ownLabels := map[string]string{constants.JobUIDLabelKey: testJobUID}
	foreignLabels := map[string]string{constants.JobUIDLabelKey: otherJobUID}

	var (
		alreadyExists = statusReply(t, apierrors.NewAlreadyExists(sparkAppResource, testSparkJobName))
		notFound      = statusReply(t, apierrors.NewNotFound(sparkAppResource, testSparkJobName))
		internal      = statusReply(t, apierrors.NewInternalError(errors.New("etcd is down")))
		forbidden     = statusReply(t, apierrors.NewForbidden(sparkAppResource, testSparkJobName, errors.New("no create permission")))

		created = sparkAppReply(t, testAppUID, ownLabels)
		own     = sparkAppReply(t, testAppUID, ownLabels)
		foreign = sparkAppReply(t, "app-uid-9", foreignLabels)

		post = "POST " + sparkCollectionPath
		get  = "GET " + sparkObjectPath
	)

	requireCreated := func(t *testing.T, err error) {
		t.Helper()
		require.NoError(t, err)
	}
	requireLaunched := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		assert.True(t, apierrors.IsAlreadyExists(err), "the job's own application must still read as AlreadyExists, got: %v", err)
		assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
	}
	// requireConflict checks for the conflict error; whose is what its message
	// must say about the application holding the name.
	requireConflict := func(whose string) func(t *testing.T, err error) {
		return func(t *testing.T, err error) {
			t.Helper()
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrSparkApplicationConflict)
			assert.False(t, apierrors.IsAlreadyExists(err), "an application that is not the job's own must not read as AlreadyExists, got: %v", err)
			// The message is what an operator reads to find the application.
			assert.Contains(t, err.Error(), "default/test-spark-job")
			assert.Contains(t, err.Error(), "test-cluster")
			assert.Contains(t, err.Error(), whose)
		}
	}
	anotherJobs := requireConflict(`belongs to job UID "` + otherJobUID + `"`)
	unlabelled := requireConflict("carries no ma/job-uid label, so Michelangelo did not create it for this job")

	tests := []struct {
		name    string
		replies []sparkReply
		// jobWithoutUID runs the create for a job whose UID is empty, which
		// owns no application, instead of the job with testJobUID.
		jobWithoutUID bool
		wantRequests  []string
		check         func(t *testing.T, err error)
	}{
		{
			name:         "no conflict: one create, nothing read",
			replies:      []sparkReply{created},
			wantRequests: []string{post},
			check:        requireCreated,
		},
		{
			name:         "the job's own application: label equal to the job UID",
			replies:      []sparkReply{alreadyExists, own},
			wantRequests: []string{post, get},
			check:        requireLaunched,
		},
		{
			// Created by hand or by another tool, outside Michelangelo: the
			// mapper labels every application it projects for a job with a
			// UID, so this one is not the job's.
			name:         "an application without the label is not the job's own",
			replies:      []sparkReply{alreadyExists, sparkAppReply(t, testAppUID, nil)},
			wantRequests: []string{post, get},
			check:        unlabelled,
		},
		{
			name:         "an application with other labels only is not the job's own",
			replies:      []sparkReply{alreadyExists, sparkAppReply(t, testAppUID, map[string]string{constants.ProjectNameLabelKey: "proj1"})},
			wantRequests: []string{post, get},
			check:        unlabelled,
		},
		{
			name:         "an application with an empty label value is not the job's own",
			replies:      []sparkReply{alreadyExists, sparkAppReply(t, testAppUID, map[string]string{constants.JobUIDLabelKey: ""})},
			wantRequests: []string{post, get},
			check:        unlabelled,
		},
		{
			// An empty UID would otherwise match an application without the label.
			name:          "a job without a UID owns no application",
			replies:       []sparkReply{alreadyExists, sparkAppReply(t, testAppUID, nil)},
			jobWithoutUID: true,
			wantRequests:  []string{post, get},
			check:         requireConflict("cannot be this job's: the job has no UID"),
		},
		{
			name:         "another job's application",
			replies:      []sparkReply{alreadyExists, foreign},
			wantRequests: []string{post, get},
			check:        anotherJobs,
		},
		{
			name:         "deleted before it could be read: the create is retried once and succeeds",
			replies:      []sparkReply{alreadyExists, notFound, created},
			wantRequests: []string{post, get, post},
			check:        requireCreated,
		},
		{
			name:         "deleted before it could be read: the retry finds the job's own application",
			replies:      []sparkReply{alreadyExists, notFound, alreadyExists, own},
			wantRequests: []string{post, get, post, get},
			check:        requireLaunched,
		},
		{
			name:         "deleted before it could be read: the retry finds another job's application",
			replies:      []sparkReply{alreadyExists, notFound, alreadyExists, foreign},
			wantRequests: []string{post, get, post, get},
			check:        anotherJobs,
		},
		{
			// Without the bound a name that keeps vanishing would spin.
			name:         "deleted before it could be read, twice: no third create",
			replies:      []sparkReply{alreadyExists, notFound, alreadyExists, notFound},
			wantRequests: []string{post, get, post, get},
			check: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsNotFound(err), "the second NotFound is the cause, got: %v", err)
				assert.False(t, apierrors.IsAlreadyExists(err))
				assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
			},
		},
		{
			name:         "the conflicting application cannot be read",
			replies:      []sparkReply{alreadyExists, internal},
			wantRequests: []string{post, get},
			check: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsInternalError(err), "the read error must stay recoverable through the wrap, got: %v", err)
				assert.False(t, apierrors.IsAlreadyExists(err), "an unread application is not known to be the job's own")
				assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
				assert.Contains(t, err.Error(), "default/test-spark-job")
			},
		},
		{
			// Only a 404 that carries a Status shows that the application was
			// deleted before it could be read. A proxy's 404 proves nothing:
			// it is a read that failed, and the create is not retried on it.
			name:         "the conflicting application's read gets a proxy's 404: no second create",
			replies:      []sparkReply{alreadyExists, proxyNotFoundReply},
			wantRequests: []string{post, get},
			check: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsUnexpectedServerError(err), "the read error must stay recoverable through the wrap, got: %v", err)
				assert.False(t, apierrors.IsAlreadyExists(err), "an unread application is not known to be the job's own")
				assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
				assert.Contains(t, err.Error(), "get spark application default/test-spark-job after create conflict")
			},
		},
		{
			name:         "the create fails for another reason: nothing is read",
			replies:      []sparkReply{forbidden},
			wantRequests: []string{post},
			check: func(t *testing.T, err error) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsForbidden(err), "got: %v", err)
				assert.False(t, apierrors.IsAlreadyExists(err))
				assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &sparkRequestRecord{}
			cl, _ := newSparkTestClient(t, scriptedSparkRESTClient(t, rec, tt.replies...), sparkMapper(t, testSparkJobName))

			job := newSparkJobWithUID(testJobUID)
			if tt.jobWithoutUID {
				job = newSparkJobWithUID("")
			}
			err := cl.CreateJob(context.Background(), job, nil,
				&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})

			tt.check(t, err)
			assert.Equal(t, tt.wantRequests, rec.lines())
		})
	}
}

// TestSparkNonStatusNotFoundReplies pins what client-go makes of the two 404
// replies without a Kubernetes Status that the rows above and below use: the
// error apierrors.NewGenericServerResponse builds for an unexpected response,
// a NotFound that is also IsUnexpectedServerError. sparkApplicationAbsent must
// not count it as proof that an application is gone, while a 404 that carries
// a Status is.
func TestSparkNonStatusNotFoundReplies(t *testing.T) {
	// The fake REST client has no group version, so the resource client-go
	// names has no group; against a member cluster it is sparkoperator.k8s.io.
	resource := schema.GroupResource{Resource: constants.KubeSparkResource}

	tests := []struct {
		name       string
		reply      sparkReply
		want       error
		wantAbsent bool
	}{
		{
			name:  "a proxy's HTML 404",
			reply: proxyNotFoundReply,
			want: apierrors.NewGenericServerResponse(http.StatusNotFound, http.MethodGet, resource, testSparkJobName,
				"<html>404 page not found</html>", 0, true),
		},
		{
			name:  "the API server's plain-text 404 for an API it does not serve",
			reply: apiNotServedReply,
			want: apierrors.NewGenericServerResponse(http.StatusNotFound, http.MethodGet, resource, testSparkJobName,
				"404 page not found", 0, true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &sparkRequestRecord{}
			err := NewHelper().GetResource(context.Background(), scriptedSparkRESTClient(t, rec, tt.reply),
				constants.KubeSparkResource, "default", testSparkJobName, &sparkv1beta2.SparkApplication{})
			assert.Equal(t, tt.want, err)
			assert.True(t, apierrors.IsNotFound(err), "got: %v", err)
			assert.True(t, apierrors.IsUnexpectedServerError(err), "got: %v", err)
			assert.False(t, sparkApplicationAbsent(err), "a 404 without a Status is not proof that the application is gone")
		})
	}

	t.Run("a 404 that carries a Status", func(t *testing.T) {
		rec := &sparkRequestRecord{}
		err := NewHelper().GetResource(context.Background(),
			scriptedSparkRESTClient(t, rec, statusReply(t, apierrors.NewNotFound(sparkAppResource, testSparkJobName))),
			constants.KubeSparkResource, "default", testSparkJobName, &sparkv1beta2.SparkApplication{})
		assert.False(t, apierrors.IsUnexpectedServerError(err), "got: %v", err)
		assert.True(t, sparkApplicationAbsent(err), "the API server said the application does not exist, got: %v", err)
	})
}

// TestCreateJobSparkJobAPINotServed: a 404 on the create's POST means the member
// cluster does not serve the SparkApplication API, because the Spark Operator is
// not installed there. Any 404 counts, with or without a Status: an API server
// answers a path that no API it serves covers with plain text.
//
// The error says so, naming the cluster and the resource, and keeps the
// original error only as text. No API status may be left in its chain: the
// SparkJob controller would record that status's message instead, "the server
// could not find the requested resource", and it retries an error whose status
// is IsUnexpectedServerError, as the plain-text 404's is, which would retry a
// creation that cannot succeed until someone installs the operator.
func TestCreateJobSparkJobAPINotServed(t *testing.T) {
	tests := []struct {
		name  string
		reply sparkReply
	}{
		{
			name: "a 404 that carries a Status",
			reply: statusReply(t, apierrors.NewGenericServerResponse(http.StatusNotFound, http.MethodPost,
				sparkAppResource, "", "", 0, false)),
		},
		{
			name:  "the API server's plain-text 404",
			reply: apiNotServedReply,
		},
		{
			// Indistinguishable from the API server's 404 on a POST to the
			// collection; the user can resubmit.
			name:  "a proxy's HTML 404",
			reply: proxyNotFoundReply,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &sparkRequestRecord{}
			cl, _ := newSparkTestClient(t, scriptedSparkRESTClient(t, rec, tt.reply), sparkMapper(t, testSparkJobName))

			err := cl.CreateJob(context.Background(), newSparkJobWithUID(testJobUID), nil,
				&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})

			require.Error(t, err)
			assert.Contains(t, err.Error(), `cluster "test-cluster" does not serve sparkapplications.sparkoperator.k8s.io`+
				` in namespace "default"; is the Spark Operator installed?`)
			assert.Contains(t, err.Error(), "(the server could not find the requested resource", "the original error is kept as text")
			var status apierrors.APIStatus
			assert.False(t, errors.As(err, &status), "no API status may be left in the chain, got: %v", err)
			assert.NotErrorIs(t, err, ErrSparkApplicationConflict)
			assert.Equal(t, []string{"POST " + sparkCollectionPath}, rec.lines(), "nothing is read and the create is not retried")
		})
	}
}

// TestCreateJobSparkJobPostsOwnershipLabelAndTTL runs the real mapper under the
// client and reads the body that goes over the wire: what the member cluster
// receives carries the job's UID in ma/job-uid and the TTL that lets the
// operator reap it. The two mapper tests prove the projection, this one
// that the client sends that projection and not some other object.
func TestCreateJobSparkJobPostsOwnershipLabelAndTTL(t *testing.T) {
	mapper := k8sengine.NewMapper(
		k8sengine.LogPersistenceConfig{},
		maconfig.SchedulerConfig{},
		env.Context{RuntimeEnvironment: "staging"},
	).Mapper

	rec := &sparkRequestRecord{}
	cl, _ := newSparkTestClient(t, sparkRESTClient(rec, http.StatusOK, sparkApplicationJSON("")), mapper)

	job := newSparkJobWithUID(testJobUID)
	// A job author's own label must not stand in for the UID.
	job.Labels = map[string]string{constants.JobUIDLabelKey: "forged-uid"}

	require.NoError(t, cl.CreateJob(context.Background(), job, nil,
		&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}))

	require.Len(t, rec.requests, 1)
	assert.Equal(t, "POST "+sparkCollectionPath, rec.lines()[0])

	var posted sparkv1beta2.SparkApplication
	require.NoError(t, json.Unmarshal([]byte(rec.requests[0].body), &posted), "POST body: %s", rec.requests[0].body)

	assert.Equal(t, "SparkApplication", posted.Kind)
	assert.Equal(t, "sparkoperator.k8s.io/v1beta2", posted.APIVersion)
	assert.Equal(t, testSparkJobName, posted.Name)
	assert.Equal(t, "default", posted.Namespace)
	assert.Equal(t, testJobUID, posted.Labels[constants.JobUIDLabelKey])
	assert.Equal(t, constants.MAOwnerSparkLabelValue, posted.Labels[constants.OwnerServiceLabelKey])
	require.NotNil(t, posted.Spec.TimeToLiveSeconds, "an application sent without a TTL is never reaped")
	assert.EqualValues(t, 86400, *posted.Spec.TimeToLiveSeconds)
}

// TestDeleteJobSparkJobOwnership pins the ownership rule (the ma/job-uid label)
// on the delete side. This is the only thing that stops a running projected
// application, and the name it deletes by is shared across projects, so a
// delete by name alone could stop another project's job.
//
// DeleteJob reads the application first. One that is not the job's own --
// another job's, or one without the job's non-empty UID in its label -- is left
// alone. The job's own is deleted with the UID of the object that was read as a
// precondition, so an application that was replaced after the read is refused
// by the API server (a 409) instead of removed.
func TestDeleteJobSparkJobOwnership(t *testing.T) {
	ownLabels := map[string]string{constants.JobUIDLabelKey: testJobUID}
	foreignLabels := map[string]string{constants.JobUIDLabelKey: otherJobUID}

	var (
		notFound = statusReply(t, apierrors.NewNotFound(sparkAppResource, testSparkJobName))
		internal = statusReply(t, apierrors.NewInternalError(errors.New("etcd is down")))
		// What an API server answers a delete whose UID precondition does not hold.
		preconditionFailed = statusReply(t, apierrors.NewConflict(sparkAppResource, testSparkJobName,
			errors.New("Precondition failed: UID in precondition: "+testAppUID+", UID in object meta: app-uid-9")))

		get = "GET " + sparkObjectPath
		del = "DELETE " + sparkObjectPath
	)

	// leftAlone checks that nothing was deleted and that the log names the
	// application and the owner its label gave ("" without one).
	leftAlone := func(owner string) func(t *testing.T, err error, logs *observer.ObservedLogs) {
		return func(t *testing.T, err error, logs *observer.ObservedLogs) {
			t.Helper()
			require.NoError(t, err, "this job's own application is gone, which is all the caller wanted")
			fields := requireInfoLog(t, logs, "is not this job's")
			assert.Equal(t, owner, fields["ownerJobUID"])
			assert.Equal(t, testSparkJobName, fields["name"])
			assert.Equal(t, "default", fields["namespace"])
			assert.Equal(t, "test-cluster", fields["cluster"])
		}
	}
	// readFailedWithoutStatus checks for the error of a read answered with a
	// 404 that carries no Status: wrapped as a failed read, with the cause the
	// caller needs to tell it from a 404 that shows the application is gone.
	readFailedWithoutStatus := func(t *testing.T, err error, _ *observer.ObservedLogs) {
		t.Helper()
		require.Error(t, err)
		assert.True(t, apierrors.IsUnexpectedServerError(err), "the read error must stay recoverable through the wrap, got: %v", err)
		assert.Contains(t, err.Error(), "get spark application default/test-spark-job before delete")
	}

	tests := []struct {
		name    string
		replies []sparkReply
		// jobWithoutUID runs the delete for a job whose UID is empty, which
		// owns no application, instead of the job with testJobUID.
		jobWithoutUID bool
		wantRequests  []string
		// wantPrecondition is the UID the DELETE must carry; "" for no DELETE.
		wantPrecondition string
		check            func(t *testing.T, err error, logs *observer.ObservedLogs)
	}{
		{
			name:         "no application: NotFound passes through and nothing is deleted",
			replies:      []sparkReply{notFound},
			wantRequests: []string{get},
			check: func(t *testing.T, err error, _ *observer.ObservedLogs) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsNotFound(err), "callers treat NotFound as success, got: %v", err)
			},
		},
		{
			name:         "another job's application is left alone",
			replies:      []sparkReply{sparkAppReply(t, "app-uid-9", foreignLabels)},
			wantRequests: []string{get},
			check:        leftAlone(otherJobUID),
		},
		{
			// Created by hand or by another tool, outside Michelangelo: deleting
			// it would stop someone else's work.
			name:         "an application without the label is left alone",
			replies:      []sparkReply{sparkAppReply(t, testAppUID, nil)},
			wantRequests: []string{get},
			check:        leftAlone(""),
		},
		{
			name:         "an application with an empty label value is left alone",
			replies:      []sparkReply{sparkAppReply(t, testAppUID, map[string]string{constants.JobUIDLabelKey: ""})},
			wantRequests: []string{get},
			check:        leftAlone(""),
		},
		{
			// An empty UID would otherwise match an application without the label.
			name:          "a job without a UID deletes nothing",
			replies:       []sparkReply{sparkAppReply(t, testAppUID, nil)},
			jobWithoutUID: true,
			wantRequests:  []string{get},
			check:         leftAlone(""),
		},
		{
			name:             "the job's own application (label equal to the job UID) is deleted by its own UID",
			replies:          []sparkReply{sparkAppReply(t, testAppUID, ownLabels), sparkDeletedReply},
			wantRequests:     []string{get, del},
			wantPrecondition: testAppUID,
			check: func(t *testing.T, err error, _ *observer.ObservedLogs) {
				t.Helper()
				require.NoError(t, err)
			},
		},
		{
			name:             "replaced between the read and the delete: the refused delete is not an error",
			replies:          []sparkReply{sparkAppReply(t, testAppUID, ownLabels), preconditionFailed},
			wantRequests:     []string{get, del},
			wantPrecondition: testAppUID,
			check: func(t *testing.T, err error, logs *observer.ObservedLogs) {
				t.Helper()
				require.NoError(t, err, "what is there now is not this job's, and this job's is gone")
				fields := requireInfoLog(t, logs, "replaced after it was read")
				assert.Equal(t, testAppUID, fields["readUID"])
			},
		},
		{
			// Only the API server's own Conflict shows the precondition was
			// refused. A proxy's 409 reads as a Conflict too, but the
			// application may still be there: taken as a refusal, it would end
			// a teardown that never happened.
			name:             "the delete gets a proxy's 409: the error is returned, not taken as a refused precondition",
			replies:          []sparkReply{sparkAppReply(t, testAppUID, ownLabels), proxyConflictReply},
			wantRequests:     []string{get, del},
			wantPrecondition: testAppUID,
			check: func(t *testing.T, err error, logs *observer.ObservedLogs) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsConflict(err), "the 409 is returned as it arrived, got: %v", err)
				assert.True(t, apierrors.IsUnexpectedServerError(err), "got: %v", err)
				assert.Zero(t, logs.FilterMessageSnippet("replaced after it was read").Len(),
					"a proxy's 409 says nothing about the application")
			},
		},
		{
			name:             "deleted between the read and the delete: NotFound passes through",
			replies:          []sparkReply{sparkAppReply(t, testAppUID, ownLabels), notFound},
			wantRequests:     []string{get, del},
			wantPrecondition: testAppUID,
			check: func(t *testing.T, err error, _ *observer.ObservedLogs) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsNotFound(err), "got: %v", err)
			},
		},
		{
			name:             "the delete fails: the error is returned, not swallowed",
			replies:          []sparkReply{sparkAppReply(t, testAppUID, ownLabels), internal},
			wantRequests:     []string{get, del},
			wantPrecondition: testAppUID,
			check: func(t *testing.T, err error, _ *observer.ObservedLogs) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsInternalError(err), "got: %v", err)
			},
		},
		{
			name:         "the application cannot be read: the error is wrapped and nothing is deleted",
			replies:      []sparkReply{internal},
			wantRequests: []string{get},
			check: func(t *testing.T, err error, _ *observer.ObservedLogs) {
				t.Helper()
				require.Error(t, err)
				assert.True(t, apierrors.IsInternalError(err), "the read error must stay recoverable through the wrap, got: %v", err)
				assert.False(t, apierrors.IsNotFound(err))
				assert.Contains(t, err.Error(), "default/test-spark-job")
			},
		},
		{
			// Only a 404 that carries a Status shows the application is gone. A
			// proxy's 404 proves nothing: it is a read that failed, returned
			// like any other.
			name:         "the read gets a proxy's 404: the error is wrapped and nothing is deleted",
			replies:      []sparkReply{proxyNotFoundReply},
			wantRequests: []string{get},
			check:        readFailedWithoutStatus,
		},
		{
			// As when the Spark Operator's CRD was removed: the application went
			// with it, but this 404 cannot be told from a proxy's, so it is a
			// read that failed like any other.
			name:         "the read gets the API server's 404 for an API it does not serve: the error is wrapped and nothing is deleted",
			replies:      []sparkReply{apiNotServedReply},
			wantRequests: []string{get},
			check:        readFailedWithoutStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &sparkRequestRecord{}
			cl, logs := newSparkTestClient(t, scriptedSparkRESTClient(t, rec, tt.replies...), sparkMapper(t, testSparkJobName))

			job := newSparkJobWithUID(testJobUID)
			if tt.jobWithoutUID {
				job = newSparkJobWithUID("")
			}
			err := cl.DeleteJob(context.Background(), job,
				&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})

			tt.check(t, err, logs)
			assert.Equal(t, tt.wantRequests, rec.lines())

			if tt.wantPrecondition == "" {
				return
			}
			var opts metav1.DeleteOptions
			deleteBody := rec.requests[len(rec.requests)-1].body
			require.NoError(t, json.Unmarshal([]byte(deleteBody), &opts), "DELETE body: %q", deleteBody)
			require.NotNil(t, opts.Preconditions, "the DELETE must carry a precondition, body: %s", deleteBody)
			require.NotNil(t, opts.Preconditions.UID, "the DELETE must carry a UID precondition, body: %s", deleteBody)
			assert.Equal(t, types.UID(tt.wantPrecondition), *opts.Preconditions.UID,
				"the precondition is the application's UID, not the job's")
		})
	}
}

// TestGetJobStatusSparkJobOwnership: the ownership rule applies to the
// status read too. The controller's safety net reads NotFound as "this job's
// own application is gone", so a same-named application that is not the job's
// own -- another job's, or one without the job's non-empty UID in its label --
// has to look absent. Handing its status over instead would report an outcome
// that is not this job's as this one's.
func TestGetJobStatusSparkJobOwnership(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		// jobUID is the UID of the job whose status is read.
		jobUID     types.UID
		wantMapped bool
	}{
		{name: "label equal to the job UID", labels: map[string]string{constants.JobUIDLabelKey: testJobUID}, jobUID: testJobUID, wantMapped: true},
		{name: "no label", labels: nil, jobUID: testJobUID, wantMapped: false},
		{name: "other labels only", labels: map[string]string{constants.ProjectNameLabelKey: "proj1"}, jobUID: testJobUID, wantMapped: false},
		{name: "empty label value", labels: map[string]string{constants.JobUIDLabelKey: ""}, jobUID: testJobUID, wantMapped: false},
		{name: "label of another job", labels: map[string]string{constants.JobUIDLabelKey: otherJobUID}, jobUID: testJobUID, wantMapped: false},
		// An empty UID would otherwise match an application without the label.
		{name: "empty job UID", labels: nil, jobUID: "", wantMapped: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper := namingOnlyMapper(t)
			if tt.wantMapped {
				mapper.EXPECT().MapLocalJobStatusToGlobal(gomock.Any()).
					DoAndReturn(func(obj runtime.Object) (*matypes.JobStatus, error) {
						app, ok := obj.(*sparkv1beta2.SparkApplication)
						require.True(t, ok, "want *sparkv1beta2.SparkApplication, got %T", obj)
						return &matypes.JobStatus{
							Spark: &v2pb.SparkJobStatus{ApplicationId: app.Status.SparkApplicationID},
						}, nil
					}).Times(1)
			}

			rec := &sparkRequestRecord{}
			cl, logs := newSparkTestClient(t,
				scriptedSparkRESTClient(t, rec, sparkAppReply(t, testAppUID, tt.labels)), mapper)

			js, err := cl.GetJobStatus(context.Background(), newSparkJobWithUID(tt.jobUID),
				&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})

			assert.Equal(t, []string{"GET " + sparkObjectPath}, rec.lines())

			if tt.wantMapped {
				require.NoError(t, err)
				require.NotNil(t, js)
				require.NotNil(t, js.Spark)
				assert.Equal(t, "spark-app-1234", js.Spark.ApplicationId)
				assert.Empty(t, logs.All(), "an application that is the job's own is not logged about")
				return
			}

			// The mapper is not consulted (namingOnlyMapper would fail the test),
			// and no status is fabricated.
			require.Error(t, err)
			assert.Nil(t, js)
			assert.True(t, apierrors.IsNotFound(err), "an application that is not the job's own must look absent, got: %v", err)

			var statusErr *apierrors.StatusError
			require.True(t, errors.As(err, &statusErr), "the NotFound is a real API status error, got: %T", err)
			assert.Equal(t, testSparkJobName, statusErr.ErrStatus.Details.Name)
			assert.Equal(t, "sparkoperator.k8s.io", statusErr.ErrStatus.Details.Group)
			assert.Equal(t, constants.KubeSparkResource, statusErr.ErrStatus.Details.Kind)

			fields := requireInfoLog(t, logs, "is not this job's")
			assert.Equal(t, tt.labels[constants.JobUIDLabelKey], fields["ownerJobUID"])
			assert.Equal(t, testSparkJobName, fields["name"])
			assert.Equal(t, "test-cluster", fields["cluster"])
		})
	}
}

// TestFederatedClientWrapsFactoryErrors: a failure to reach the compute
// cluster is wrapped with %w, so the cause stays reachable with errors.Is. A %v
// would flatten it into text, and the callers that tell a transient cause from a
// terminal one by its type would see every such failure as terminal.
func TestFederatedClientWrapsFactoryErrors(t *testing.T) {
	ctx := context.Background()
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}
	factoryErr := errors.New("factory: cluster unreachable")

	calls := []struct {
		name string
		call func(c *Client) error
	}{
		{"GetJobStatus/ray", func(c *Client) error {
			_, err := c.GetJobStatus(ctx, &v2pb.RayJob{}, cluster)
			return err
		}},
		{"GetJobStatus/spark", func(c *Client) error {
			_, err := c.GetJobStatus(ctx, newSparkJobWithUID(testJobUID), cluster)
			return err
		}},
		{"CreateJob/ray", func(c *Client) error {
			return c.CreateJob(ctx, &v2pb.RayJob{}, &v2pb.RayCluster{}, cluster)
		}},
		{"CreateJob/spark", func(c *Client) error {
			return c.CreateJob(ctx, newSparkJobWithUID(testJobUID), nil, cluster)
		}},
		{"CreateJobCluster", func(c *Client) error {
			return c.CreateJobCluster(ctx, &v2pb.RayCluster{}, cluster)
		}},
		{"DeleteJobCluster", func(c *Client) error {
			return c.DeleteJobCluster(ctx, &v2pb.RayCluster{}, cluster)
		}},
		{"GetJobClusterStatus", func(c *Client) error {
			_, err := c.GetJobClusterStatus(ctx, &v2pb.RayCluster{}, cluster)
			return err
		}},
	}

	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			f := computemocks.NewMockFactory(gomock.NewController(t))
			f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, factoryErr)
			cl := &Client{factory: f, helper: NewHelper(), mapper: permissiveMapper(t), logger: zaptest.NewLogger(t)}

			err := tt.call(cl)

			require.Error(t, err)
			assert.ErrorIs(t, err, factoryErr, "the factory's own error must stay reachable through the wrap")
			assert.Contains(t, err.Error(), "get client for cluster err")
		})
	}
}

// TestSparkArmsWrapMapperErrors: a mapping failure is wrapped with %w too, so a
// caller that distinguishes causes can still see the mapper's.
func TestSparkArmsWrapMapperErrors(t *testing.T) {
	ctx := context.Background()
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}
	mapErr := errors.New("mapper: cannot project")

	t.Run("CreateJob maps before anything is sent", func(t *testing.T) {
		mapper := namingOnlyMapper(t)
		mapper.EXPECT().MapGlobalJobToLocal(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, mapErr)
		rec := &sparkRequestRecord{}
		cl, _ := newSparkTestClient(t, scriptedSparkRESTClient(t, rec), mapper)

		err := cl.CreateJob(ctx, newSparkJobWithUID(testJobUID), nil, cluster)

		require.Error(t, err)
		assert.ErrorIs(t, err, mapErr)
		assert.Zero(t, rec.calls, "a job that cannot be projected sends nothing")
	})

	t.Run("GetJobStatus", func(t *testing.T) {
		mapper := namingOnlyMapper(t)
		mapper.EXPECT().MapLocalJobStatusToGlobal(gomock.Any()).Return(nil, mapErr)
		rec := &sparkRequestRecord{}
		// The job's own application, or GetJobStatus reports it absent before
		// it maps anything.
		own := sparkAppReply(t, testAppUID, map[string]string{constants.JobUIDLabelKey: testJobUID})
		cl, _ := newSparkTestClient(t, scriptedSparkRESTClient(t, rec, own), mapper)

		js, err := cl.GetJobStatus(ctx, newSparkJobWithUID(testJobUID), cluster)

		require.Error(t, err)
		assert.Nil(t, js)
		assert.ErrorIs(t, err, mapErr)
	})
}

// TestSparkArmsRefuseMissingSparkClient: a compute cluster whose client
// set has no SparkApplication client is an error in every Spark arm. The
// production factory always fills it in, but another compute.Factory may not,
// and a nil rest.Interface would otherwise panic at the first request.
func TestSparkArmsRefuseMissingSparkClient(t *testing.T) {
	ctx := context.Background()
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	arms := []struct {
		name string
		call func(c *Client) error
	}{
		{"CreateJob", func(c *Client) error {
			return c.CreateJob(ctx, newSparkJobWithUID(testJobUID), nil, cluster)
		}},
		{"DeleteJob", func(c *Client) error {
			return c.DeleteJob(ctx, newSparkJobWithUID(testJobUID), cluster)
		}},
		{"GetJobStatus", func(c *Client) error {
			_, err := c.GetJobStatus(ctx, newSparkJobWithUID(testJobUID), cluster)
			return err
		}},
	}

	clientSets := []struct {
		name string
		set  func(t *testing.T) *compute.ClientSet
	}{
		{"ClientSet.Spark is nil", func(t *testing.T) *compute.ClientSet {
			return &compute.ClientSet{Ray: forbiddenRESTClient(t), CoreV1: forbiddenRESTClient(t)}
		}},
		{"the factory returns no ClientSet", func(*testing.T) *compute.ClientSet { return nil }},
	}

	for _, arm := range arms {
		for _, cs := range clientSets {
			t.Run(arm.name+"/"+cs.name, func(t *testing.T) {
				f := computemocks.NewMockFactory(gomock.NewController(t))
				f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(cs.set(t), nil)
				cl := &Client{factory: f, helper: NewHelper(), mapper: sparkMapper(t, testSparkJobName), logger: zaptest.NewLogger(t)}

				var err error
				require.NotPanics(t, func() { err = arm.call(cl) })

				require.Error(t, err)
				assert.ErrorIs(t, err, errNoSparkClient)
				assert.Contains(t, err.Error(), "test-cluster", "the error names the cluster")
			})
		}
	}
}

// TestJobArmsRejectUnknownJobTypes: an object that is not a job is an error from
// every job arm, never a silent success. CreateJob used to return nil for it,
// which a caller reads as "launched". The factory has no expectations, so
// reaching a cluster for an object that is not a job fails the test as well.
func TestJobArmsRejectUnknownJobTypes(t *testing.T) {
	ctx := context.Background()
	cluster := &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}}

	// A RayCluster is a job-cluster object, not a job.
	for name, obj := range map[string]runtime.Object{
		"a job-cluster object": &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc1"}},
		"nil":                  nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := computemocks.NewMockFactory(gomock.NewController(t))
			cl := &Client{factory: f, helper: NewHelper(), mapper: permissiveMapper(t), logger: zaptest.NewLogger(t)}

			require.Error(t, cl.CreateJob(ctx, obj, nil, cluster), "CreateJob")
			require.Error(t, cl.DeleteJob(ctx, obj, cluster), "DeleteJob")
			js, err := cl.GetJobStatus(ctx, obj, cluster)
			require.Error(t, err, "GetJobStatus")
			assert.Nil(t, js)
		})
	}
}

// watcherSpyHelper is a Helper whose Watcher records what it was handed and
// builds nothing. The embedded Helper is nil, so any other method panics: the
// Watcher path must use none of them.
type watcherSpyHelper struct {
	Helper

	calls int
	got   []*WatcherParams
}

func (h *watcherSpyHelper) Watcher(params []*WatcherParams) ([]*ResourceWatcher, error) {
	h.calls++
	h.got = params
	return []*ResourceWatcher{{}}, nil
}

// TestWatcherSparkArm: the Spark watch is built on the compute cluster's
// Spark client. Client.Watcher mutates the params in place, so the client each
// one ended up with is checked by identity, against distinct Ray, Spark and
// CoreV1 clients: a Spark arm that picked the Ray client up (they are all the
// same interface type) would otherwise go unnoticed.
func TestWatcherSparkArm(t *testing.T) {
	rayClient := forbiddenRESTClient(t)
	sparkClient := forbiddenRESTClient(t)
	coreClient := forbiddenRESTClient(t)

	f := computemocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:    rayClient,
		Spark:  sparkClient,
		CoreV1: coreClient,
	}, nil)
	spy := &watcherSpyHelper{}
	cl := &Client{factory: f, helper: spy, logger: zaptest.NewLogger(t)}

	params := []*WatcherParams{
		{ResourceName: constants.KubeRayResource},
		{ResourceName: constants.KubeRayJobResource},
		{ResourceName: v1.ResourcePods.String()},
		{ResourceName: v1.ResourceConfigMaps.String()},
		{ResourceName: constants.KubeSparkResource},
	}

	_, err := cl.Watcher(params, &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})
	require.NoError(t, err)

	require.Same(t, rayClient, params[0].Client, "rayclusters")
	require.Same(t, rayClient, params[1].Client, "rayjobs")
	require.Same(t, coreClient, params[2].Client, "pods")
	require.Same(t, coreClient, params[3].Client, "configmaps")
	require.Same(t, sparkClient, params[4].Client, "sparkapplications must watch through the Spark client")

	require.Equal(t, 1, spy.calls, "the helper builds all the watchers in one call")
	require.Len(t, spy.got, len(params))
	assert.Same(t, params[4], spy.got[4], "the helper is handed the params the client completed")
}

// TestWatcherSparkArmRefusesMissingSparkClient: a missing Spark client is an
// error on the watch path too. A nil client there is the worst case: it is
// accepted silently and the informer's reflector goroutine panics later, where
// nothing recovers it and the process dies.
func TestWatcherSparkArmRefusesMissingSparkClient(t *testing.T) {
	f := computemocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{
		Ray:    forbiddenRESTClient(t),
		CoreV1: forbiddenRESTClient(t),
	}, nil)
	spy := &watcherSpyHelper{}
	cl := &Client{factory: f, helper: spy, logger: zaptest.NewLogger(t)}

	res, err := cl.Watcher(
		[]*WatcherParams{{ResourceName: constants.KubeSparkResource}},
		&v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster"}})

	require.Error(t, err)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, errNoSparkClient)
	assert.Zero(t, spy.calls, "no watcher is built on a missing client")
}
