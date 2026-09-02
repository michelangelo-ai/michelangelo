package kueue

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	restfake "k8s.io/client-go/rest/fake"

	maconfig "github.com/michelangelo-ai/michelangelo/go/base/config"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

func fakeFactory(g *gomock.Controller, status int, gotPath *string) compute.Factory {
	restClient := &restfake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         schema.GroupVersion{},
		Client: restfake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			*gotPath = req.URL.Path
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       http.NoBody,
			}, nil
		}),
	}

	factory := computemocks.NewMockFactory(g)
	factory.EXPECT().GetClientSetForCluster(gomock.Any()).
		Return(&compute.ClientSet{CoreV1: restClient}, nil).AnyTimes()
	return factory
}

func TestLocalQueuesExists(t *testing.T) {
	g := gomock.NewController(t)
	var gotPath string
	lq := NewLocalQueues(fakeFactory(g, http.StatusOK, &gotPath), maconfig.SchedulerConfig{})

	exists, err := lq.Exists(context.Background(), &v2pb.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}}, "default", "ma-proj1")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "/apis/kueue.x-k8s.io/v1beta2/namespaces/default/localqueues/ma-proj1", gotPath)
}

func TestLocalQueuesNotFound(t *testing.T) {
	g := gomock.NewController(t)
	var gotPath string
	lq := NewLocalQueues(fakeFactory(g, http.StatusNotFound, &gotPath), maconfig.SchedulerConfig{})

	exists, err := lq.Exists(context.Background(), &v2pb.Cluster{}, "default", "missing")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestLocalQueuesServerError(t *testing.T) {
	g := gomock.NewController(t)
	var gotPath string
	lq := NewLocalQueues(fakeFactory(g, http.StatusInternalServerError, &gotPath), maconfig.SchedulerConfig{})

	_, err := lq.Exists(context.Background(), &v2pb.Cluster{}, "default", "q")
	require.Error(t, err)
}

func TestLocalQueuesAPIVersionOverride(t *testing.T) {
	g := gomock.NewController(t)
	var gotPath string
	cfg := maconfig.SchedulerConfig{Kueue: maconfig.KueueConfig{APIVersion: "v1beta1"}}
	lq := NewLocalQueues(fakeFactory(g, http.StatusOK, &gotPath), cfg)

	_, err := lq.Exists(context.Background(), &v2pb.Cluster{}, "default", "q")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(gotPath, "/apis/kueue.x-k8s.io/v1beta1/"))
}
