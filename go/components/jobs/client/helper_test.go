package client

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	"github.com/michelangelo-ai/michelangelo/go/components/spark/sparkoperator"
	sparkv1beta2 "github.com/michelangelo-ai/michelangelo/go/thirdparty/k8s-crds/apis/sparkoperator.k8s.io/v1beta2"
	rayv1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientfeatures "k8s.io/client-go/features"
	clientfeaturestesting "k8s.io/client-go/features/testing"
	"k8s.io/client-go/rest/fake"
	"k8s.io/client-go/tools/cache"
)

// defaultHelper.Watcher used to answer `Spark job is not supported` for
// sparkapplications, and it returns on that error rather than skipping the
// entry — so one Spark watch request used to fail the whole batch, taking the
// caller's unrelated watches down with it. These two cases pin the fix: the
// Spark arm builds a ListWatch like every other CRD arm, and a batch that
// contains one is no longer special.
//
// Nothing here dials. NewFilteredListWatchFromClient only closes over the
// rest.Interface, and cache.NewInformer only constructs a DeltaFIFO and a
// Config — the client is first touched when the returned Controller is run,
// which these tests deliberately never do. A zero-value fake.RESTClient is
// therefore a sufficient stand-in.

// sparkWatcherParams is what the Spark job watcher asks for, reduced to the
// fields defaultHelper.Watcher actually reads.
func sparkWatcherParams() *WatcherParams {
	return &WatcherParams{
		Client:       &fake.RESTClient{},
		ResourceName: constants.KubeSparkResource,
		Namespace:    "default",
		ObjType:      &sparkv1beta2.SparkApplication{},
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.OwnerServiceLabelKey: constants.MAOwnerSparkLabelValue,
			},
		},
		ResourceEventHandler: cache.ResourceEventHandlerFuncs{},
	}
}

// podWatcherParams takes the `default` arm of the switch, so a batch holding
// both proves the Spark arm no longer aborts its neighbours.
func podWatcherParams() *WatcherParams {
	return &WatcherParams{
		Client:               &fake.RESTClient{},
		ResourceName:         "pods",
		Namespace:            "default",
		ObjType:              &corev1.Pod{},
		ResourceEventHandler: cache.ResourceEventHandlerFuncs{},
	}
}

func requireUsableWatchers(t *testing.T, watchers []*ResourceWatcher, want int) {
	t.Helper()
	require.Len(t, watchers, want)
	for i, w := range watchers {
		require.NotNil(t, w, "watcher %d", i)
		require.NotNil(t, w.Store, "watcher %d store", i)
		require.NotNil(t, w.Controller, "watcher %d controller", i)
	}
}

func TestHelperWatcherSparkMixedBatch(t *testing.T) {
	watchers, err := NewHelper().Watcher([]*WatcherParams{podWatcherParams(), sparkWatcherParams()})

	require.NoError(t, err)
	requireUsableWatchers(t, watchers, 2)
}

func TestHelperWatcherSparkOnly(t *testing.T) {
	watchers, err := NewHelper().Watcher([]*WatcherParams{sparkWatcherParams()})

	require.NoError(t, err)
	requireUsableWatchers(t, watchers, 1)
}

// rayClusterWatcherParams takes the kuberay.ParameterCodec arm of the switch,
// the arm the Ray cluster watch actually uses.
func rayClusterWatcherParams() *WatcherParams {
	return &WatcherParams{
		Client:       &fake.RESTClient{},
		ResourceName: constants.KubeRayResource,
		Namespace:    "default",
		ObjType:      &rayv1.RayCluster{},
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				constants.OwnerServiceLabelKey: constants.MAOwnerServiceLabelValue,
			},
		},
		ResourceEventHandler: cache.ResourceEventHandlerFuncs{},
	}
}

// TestHelperWatcherRayAndSparkBatch: a batch holding a Ray CRD watch and a
// Spark CRD watch yields both watchers. The mixed batch above uses a pods param
// as the Spark param's neighbour, which takes the `default` arm; this one makes
// the neighbour a real Ray CRD watch, so a Spark param cannot poison the batch
// whichever arm its neighbour takes.
func TestHelperWatcherRayAndSparkBatch(t *testing.T) {
	watchers, err := NewHelper().Watcher([]*WatcherParams{rayClusterWatcherParams(), sparkWatcherParams()})

	require.NoError(t, err)
	requireUsableWatchers(t, watchers, 2)
}

// recordingWatchClient is a fake Spark REST client for the informer the Spark
// arm builds. It records every request the informer sends, answers a LIST with
// an empty SparkApplicationList and holds every WATCH open until release is
// closed. A streaming list, a WATCH with sendInitialEvents=true, first gets the
// bookmark that ends its initial events, of which there are none. It speaks the
// sparkoperator group version through the sparkoperator codecs, the way the
// real client does, so a codec that cannot encode the request fails here as it
// would in production.
type recordingWatchClient struct {
	mu       sync.Mutex
	requests []watchRequest
	release  chan struct{}
}

// watchRequest is one request the informer sent.
type watchRequest struct {
	method string
	path   string
	query  url.Values
}

const emptySparkApplicationList = `{"apiVersion":"sparkoperator.k8s.io/v1beta2","kind":"SparkApplicationList",` +
	`"metadata":{"resourceVersion":"1"},"items":[]}`

// initialEventsEndBookmark is the event an API server ends a streaming list's
// initial events with: a BOOKMARK whose object carries the
// k8s.io/initial-events-end annotation and the list's resourceVersion.
const initialEventsEndBookmark = `{"type":"BOOKMARK","object":{"apiVersion":"sparkoperator.k8s.io/v1beta2",` +
	`"kind":"SparkApplication","metadata":{"resourceVersion":"1",` +
	`"annotations":{"k8s.io/initial-events-end":"true"}}}}` + "\n"

func newRecordingWatchClient() *recordingWatchClient {
	return &recordingWatchClient{release: make(chan struct{})}
}

func (r *recordingWatchClient) restClient() *fake.RESTClient {
	return &fake.RESTClient{
		GroupVersion:         sparkoperator.SchemeGroupVersion,
		NegotiatedSerializer: sparkoperator.Codecs.WithoutConversion(),
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			r.mu.Lock()
			r.requests = append(r.requests, watchRequest{
				method: req.Method,
				path:   req.URL.Path,
				query:  req.URL.Query(),
			})
			r.mu.Unlock()

			header := http.Header{}
			header.Set("Content-Type", runtime.ContentTypeJSON)
			if req.URL.Query().Get("watch") != "true" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     header,
					Body:       io.NopCloser(strings.NewReader(emptySparkApplicationList)),
				}, nil
			}

			// A watch stream with no events, open until the test is over. A
			// write blocks until the informer reads it, and fails once the
			// informer has stopped and closed the body.
			body, stream := io.Pipe()
			streamingList := req.URL.Query().Get("sendInitialEvents") == "true"
			go func() {
				if streamingList {
					_, _ = io.WriteString(stream, initialEventsEndBookmark)
				}
				<-r.release
				_ = stream.Close()
			}()
			return &http.Response{StatusCode: http.StatusOK, Header: header, Body: body}, nil
		}),
	}
}

// waitForRequests returns the first n requests, waiting for them to arrive.
func (r *recordingWatchClient) waitForRequests(t *testing.T, n int) []watchRequest {
	t.Helper()
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.requests) >= n
	}, 10*time.Second, 5*time.Millisecond, "the informer sent fewer than %d requests", n)

	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]watchRequest(nil), r.requests[:n]...)
}

// TestHelperWatcherSparkRequestShape runs the informer the Spark arm builds
// against a recording fake and reads what it actually sends. The
// structural tests above never run the list-watch, so these all passed
// unnoticed:
//
//   - a Spark arm built on kuberay.ParameterCodec, whose scheme has no
//     ListOptions for the sparkoperator group version: the first LIST cannot be
//     encoded, the informer never syncs, and the federated watcher silently
//     produces no Spark status;
//   - a dropped label selector: the control plane would watch every
//     SparkApplication in the member namespace, not only those it projected.
//
// client-go lists by streaming by default (its WatchListClient feature): the
// reflector's first request is a WATCH with sendInitialEvents=true, on which
// the API server sends the existing objects as events and then a bookmark
// that ends them. With the feature off, and against an API server that does
// not stream lists, the reflector lists first and then watches from the list's
// resourceVersion. A subtest pins each.
func TestHelperWatcherSparkRequestShape(t *testing.T) {
	const (
		wantPath     = "/namespaces/default/sparkapplications"
		wantSelector = "ma/owner-service=michelangelo-spark"
	)

	t.Run("streaming list", func(t *testing.T) {
		clientfeaturestesting.SetFeatureDuringTest(t, clientfeatures.WatchListClient, true)
		rec := newRecordingWatchClient()
		informer := runSparkInformer(t, rec)

		list := rec.waitForRequests(t, 1)[0]
		assert.Equal(t, http.MethodGet, list.method)
		assert.Equal(t, wantPath, list.path)
		assert.Equal(t, "true", list.query.Get("watch"), "a streaming list is a WATCH")
		assert.Equal(t, "true", list.query.Get("sendInitialEvents"), "a streaming list asks for the existing objects as events")
		assert.Equal(t, wantSelector, list.query.Get("labelSelector"), "the streaming list must carry the label selector")
		assert.Eventually(t, informer.HasSynced, 10*time.Second, 5*time.Millisecond,
			"the informer did not sync on the bookmark that ends the initial events")
	})

	t.Run("list then watch", func(t *testing.T) {
		clientfeaturestesting.SetFeatureDuringTest(t, clientfeatures.WatchListClient, false)
		rec := newRecordingWatchClient()
		runSparkInformer(t, rec)

		requests := rec.waitForRequests(t, 2)
		list, watch := requests[0], requests[1]

		assert.Equal(t, http.MethodGet, list.method)
		assert.Equal(t, wantPath, list.path)
		assert.Equal(t, wantSelector, list.query.Get("labelSelector"), "the LIST must carry the label selector")
		assert.NotEqual(t, "true", list.query.Get("watch"), "the first request is the LIST")

		assert.Equal(t, http.MethodGet, watch.method)
		assert.Equal(t, wantPath, watch.path)
		assert.Equal(t, "true", watch.query.Get("watch"), "the second request is the WATCH")
		assert.Equal(t, wantSelector, watch.query.Get("labelSelector"), "the WATCH must carry the label selector")
		assert.Equal(t, "1", watch.query.Get("resourceVersion"), "the WATCH resumes from the LIST's resourceVersion")
	})
}

// runSparkInformer runs the informer the Spark arm builds on rec's client
// until the test ends, and returns its controller.
func runSparkInformer(t *testing.T, rec *recordingWatchClient) cache.Controller {
	t.Helper()
	params := sparkWatcherParams()
	params.Client = rec.restClient()

	watchers, err := NewHelper().Watcher([]*WatcherParams{params})
	require.NoError(t, err)
	requireUsableWatchers(t, watchers, 1)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchers[0].Controller.Run(stop)
	}()
	t.Cleanup(func() {
		close(stop)
		close(rec.release)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the informer did not stop")
		}
	})
	return watchers[0].Controller
}

// TestHelperWatcherSparkRefusesNilClient: a Spark watch with no client is an
// error up front. Left to the informer it would panic in the reflector's own
// goroutine, which nothing recovers.
func TestHelperWatcherSparkRefusesNilClient(t *testing.T) {
	params := sparkWatcherParams()
	params.Client = nil

	var (
		watchers []*ResourceWatcher
		err      error
	)
	require.NotPanics(t, func() {
		watchers, err = NewHelper().Watcher([]*WatcherParams{podWatcherParams(), params})
	})

	require.Error(t, err)
	assert.Nil(t, watchers, "a batch with a client-less Spark watch builds nothing")
	assert.Contains(t, err.Error(), constants.KubeSparkResource)
}
