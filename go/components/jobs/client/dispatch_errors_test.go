package client

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/secrets"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/types/typesmocks"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/compute/computemocks"
	"github.com/michelangelo-ai/michelangelo/go/components/ray/kuberay"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var rayClustersResource = schema.GroupResource{Group: "ray.io", Resource: "rayclusters"}

func statusError(code int32, reason metav1.StatusReason, message string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure, Code: code, Reason: reason, Message: message,
	}}
}

// TestClassifyDispatchError walks RFC 0f's table row by row, in precedence
// order.
func TestClassifyDispatchError(t *testing.T) {
	dialErr := &url.Error{Op: "Post", URL: "https://compute:6443", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}}
	dnsErr := &url.Error{Op: "Post", URL: "https://compute:6443", Err: &net.DNSError{Err: "no such host", Name: "compute", IsNotFound: true}}
	deadline := &url.Error{Op: "Post", URL: "https://compute:6443", Err: context.DeadlineExceeded}
	x509Err := &url.Error{Op: "Post", URL: "https://compute:6443", Err: x509.UnknownAuthorityError{}}
	connectTimeout := fmt.Errorf("%w: %w", errConnectTimeout, &url.Error{Op: "Post", URL: "https://compute:6443", Err: context.Canceled})
	reset := &url.Error{Op: "Post", URL: "https://compute:6443", Err: &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}}
	writeErr := &url.Error{Op: "Post", URL: "https://compute:6443", Err: &net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}}
	webhookDown := apierrors.NewInternalError(errors.New(`failed calling webhook "mraycluster.kb.io": failed to call webhook: Post "https://kueue-webhook-service.kueue-system.svc:443/mutate-ray-io-v1-raycluster": dial tcp 10.0.0.1:443: connect: connection refused`))

	noConn := requestTrace{}
	connected := requestTrace{gotConn: true}
	writeFailed := requestTrace{gotConn: true, writeFailed: true}

	tests := []struct {
		name  string
		err   error
		trace requestTrace
		want  DispatchClass
	}{
		{name: "nil", err: nil, trace: connected, want: DispatchSuccess},
		{name: "already exists", err: apierrors.NewAlreadyExists(rayClustersResource, "rc"), trace: connected, want: DispatchAlreadyExists},
		{name: "generic 409 on POST is already exists", err: apierrors.NewGenericServerResponse(http.StatusConflict, "POST", rayClustersResource, "rc", "", 0, false), trace: connected, want: DispatchAlreadyExists},

		{name: "400 bad request", err: apierrors.NewBadRequest("bad"), trace: connected, want: DispatchPermanent},
		{name: "422 invalid", err: apierrors.NewInvalid(schema.GroupKind{Group: "ray.io", Kind: "RayCluster"}, "rc", nil), trace: connected, want: DispatchPermanent},
		{name: "403 webhook denial", err: apierrors.NewForbidden(rayClustersResource, "rc", errors.New(`admission webhook "vraycluster.kb.io" denied the request`)), trace: connected, want: DispatchPermanent},
		{name: "spec mapping error before any request", err: fmt.Errorf("map global to local err:%w", markError(errors.New("bad spec"), ErrJobSpecMapping)), trace: noConn, want: DispatchPermanent},

		{name: "no connection: dial refused", err: dialErr, trace: noConn, want: DispatchUnreached},
		{name: "no connection: DNS", err: dnsErr, trace: noConn, want: DispatchUnreached},
		{name: "no connection: deadline", err: deadline, trace: noConn, want: DispatchUnreached},
		{name: "no connection: x509", err: x509Err, trace: noConn, want: DispatchUnreached},
		{name: "no connection: connect bound", err: connectTimeout, trace: noConn, want: DispatchUnreached},
		{name: "no connection: whatever the error", err: errors.New("something odd"), trace: noConn, want: DispatchUnreached},
		{name: "connected, write failed", err: writeErr, trace: writeFailed, want: DispatchUnreached},
		{name: "client unavailable", err: fmt.Errorf("get client for cluster err:%w", markError(errors.New("secret not found"), ErrClusterClientUnavailable)), trace: connected, want: DispatchUnreached},

		{name: "401", err: apierrors.NewUnauthorized("token expired"), trace: connected, want: DispatchRejected},
		{name: "404", err: apierrors.NewNotFound(rayClustersResource, "rc"), trace: connected, want: DispatchRejected},
		{name: "409 conflict", err: apierrors.NewConflict(rayClustersResource, "rc", errors.New("modified")), trace: connected, want: DispatchRejected},
		{name: "429", err: apierrors.NewTooManyRequests("slow down", 0), trace: connected, want: DispatchRejected},
		{name: "another 4xx", err: statusError(http.StatusGone, metav1.StatusReasonGone, "gone"), trace: connected, want: DispatchRejected},
		{name: "503", err: apierrors.NewServiceUnavailable("unavailable"), trace: connected, want: DispatchRejected},
		{name: "webhook 500", err: webhookDown, trace: connected, want: DispatchRejected},
		// A response proves the server was reached, whatever the trace says.
		{name: "a response is never unreached: 503", err: apierrors.NewServiceUnavailable("unavailable"), trace: noConn, want: DispatchRejected},
		{name: "a response is never unreached: 504", err: apierrors.NewTimeoutError("timed out", 0), trace: writeFailed, want: DispatchAmbiguous},

		{name: "408", err: statusError(http.StatusRequestTimeout, metav1.StatusReasonTimeout, "request timeout"), trace: connected, want: DispatchAmbiguous},
		{name: "504 timeout", err: apierrors.NewTimeoutError("timed out", 0), trace: connected, want: DispatchAmbiguous},
		{name: "server timeout", err: apierrors.NewServerTimeout(rayClustersResource, "create", 0), trace: connected, want: DispatchAmbiguous},
		{name: "other 500", err: apierrors.NewInternalError(errors.New("etcd leader changed")), trace: connected, want: DispatchAmbiguous},
		{name: "502", err: statusError(http.StatusBadGateway, "", "bad gateway"), trace: connected, want: DispatchAmbiguous},
		{name: "connected then deadline", err: deadline, trace: connected, want: DispatchAmbiguous},
		{name: "connected then reset", err: reset, trace: connected, want: DispatchAmbiguous},
		{name: "connected then EOF", err: &url.Error{Op: "Post", URL: "https://compute:6443", Err: io.EOF}, trace: connected, want: DispatchAmbiguous},
		{name: "connected then unrecognized", err: errors.New("something odd"), trace: connected, want: DispatchAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyDispatchError(tt.err, tt.trace))
			if tt.err != nil {
				// The same, through a traced request's error.
				assert.Equal(t, tt.want, ClassifyDispatchError(fmt.Errorf("create ray cluster err:%w", &requestError{err: tt.err, trace: tt.trace})))
			}
		})
	}
}

func TestClassifyDispatchError_UntracedErrorCountsAsReached(t *testing.T) {
	assert.Equal(t, DispatchAmbiguous, ClassifyDispatchError(errors.New("something odd")))
	assert.Equal(t, DispatchRejected, ClassifyDispatchError(apierrors.NewServiceUnavailable("unavailable")))
	assert.Equal(t, DispatchUnreached, ClassifyDispatchError(fmt.Errorf("x: %w", ErrClusterClientUnavailable)))
}

func TestDispatchClassString(t *testing.T) {
	assert.Equal(t, "success", DispatchSuccess.String())
	assert.Equal(t, "already_exists", DispatchAlreadyExists.String())
	assert.Equal(t, "permanent", DispatchPermanent.String())
	assert.Equal(t, "unreached", DispatchUnreached.String())
	assert.Equal(t, "rejected", DispatchRejected.String())
	assert.Equal(t, "ambiguous", DispatchAmbiguous.String())
	assert.Equal(t, "unknown", DispatchClass(99).String())
}

func TestIsCredentialError(t *testing.T) {
	assert.True(t, isCredentialError(apierrors.NewUnauthorized("expired")))
	assert.True(t, isCredentialError(&url.Error{Op: "Post", Err: x509.UnknownAuthorityError{}}))
	assert.True(t, isCredentialError(x509.HostnameError{}))
	assert.True(t, isCredentialError(x509.CertificateInvalidError{}))
	assert.False(t, isCredentialError(nil))
	assert.False(t, isCredentialError(apierrors.NewForbidden(rayClustersResource, "rc", errors.New("denied"))))
	assert.False(t, isCredentialError(errors.New("connection refused")))
}

func TestMarkErrorKeepsMessage(t *testing.T) {
	inner := errors.New("dummy error")
	err := fmt.Errorf("get client for cluster err:%w", markError(inner, ErrClusterClientUnavailable))
	assert.Equal(t, "get client for cluster err:dummy error", err.Error())
	assert.ErrorIs(t, err, ErrClusterClientUnavailable)
	assert.ErrorIs(t, err, inner)
	assert.NotErrorIs(t, err, ErrJobSpecMapping)
}

// TestComputeRestConfigHasNoDial: client-go caches transports keyed on a
// config's Dial, which is a new pointer on every build, so a Dial on the
// compute config would leak one transport per client rebuild. The connect
// bound lives on each request instead.
func TestComputeRestConfigHasNoDial(t *testing.T) {
	cfg, err := compute.GetKubeClientConfigFromConfiguration("michelangelo", "https://compute-a:6443", metav1.NamespaceAll, &secrets.ClientAuth{}, true)
	require.NoError(t, err)
	assert.Nil(t, cfg.Dial)
}

// writeStatus answers like the API server does.
func writeStatus(w http.ResponseWriter, code int, reason metav1.StatusReason, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(&metav1.Status{
		TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
		Status:   metav1.StatusFailure,
		Code:     int32(code),
		Reason:   reason,
		Message:  message,
	})
}

// h2Server serves handler over TLS with HTTP/2, as compute API servers do,
// and fails the test on a request over another protocol.
func h2Server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("request over %s, want HTTP/2", r.Proto)
		}
		handler(w, r)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

// trustingConfig returns a config for server that trusts its certificate.
func trustingConfig(server *httptest.Server) *rest.Config {
	caData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	return &rest.Config{Host: server.URL, TLSClientConfig: rest.TLSClientConfig{CAData: caData}}
}

// statusServer reads each request in full, then answers with a Status.
func statusServer(t *testing.T, code int, reason metav1.StatusReason, message string) *rest.Config {
	return trustingConfig(h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		writeStatus(w, code, reason, message)
	}))
}

// stallingServer reads each request in full and then never answers.
func stallingServer(t *testing.T) *rest.Config {
	return trustingConfig(stallingH2Server(t, nil))
}

// stallingH2Server answers with answer when it is set and returns true, and
// otherwise reads the request in full and stalls until the client gives up.
func stallingH2Server(t *testing.T, answer func(w http.ResponseWriter, r *http.Request) bool) *httptest.Server {
	release := make(chan struct{})
	server := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		if answer != nil && answer(w, r) {
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	// Runs before the server's Close, which waits for open requests.
	t.Cleanup(func() { close(release) })
	return server
}

// silentTLSListener accepts TCP connections and never answers, so a TLS
// client stalls in the handshake and never gets a connection.
func silentTLSListener(t *testing.T) *rest.Config {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
	return &rest.Config{Host: "https://" + listener.Addr().String(), TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
}

func refusedConfig(t *testing.T) *rest.Config {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return &rest.Config{Host: "https://" + addr, TLSClientConfig: rest.TLSClientConfig{Insecure: true}}
}

// untrustedTLSServer presents a certificate the client does not trust.
func untrustedTLSServer(t *testing.T) *rest.Config {
	server := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusCreated)
	})
	return &rest.Config{Host: server.URL}
}

// withConnectBound shortens the connect bound for one test.
func withConnectBound(t *testing.T, bound time.Duration) {
	saved := connectBound
	connectBound = bound
	t.Cleanup(func() { connectBound = saved })
}

var dispatchTestCluster = &v2pb.Cluster{
	ObjectMeta: metav1.ObjectMeta{Name: "compute-a"},
	Spec: v2pb.ClusterSpec{Cluster: &v2pb.ClusterSpec_Kubernetes{Kubernetes: &v2pb.KubernetesSpec{
		Rest: &v2pb.ConnectionSpec{Host: "https://compute-a", Port: "6443"},
	}}},
}

// clientFor returns a Client whose factory hands out a real Ray REST client
// for cfg, expecting Invalidate only when invalidate is set.
func clientFor(t *testing.T, cfg *rest.Config, invalidate bool) *Client {
	rayClient, err := kuberay.NewRestClient(cfg)
	require.NoError(t, err)
	factory := computemocks.NewMockFactory(gomock.NewController(t))
	factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{Ray: rayClient}, nil).AnyTimes()
	if invalidate {
		factory.EXPECT().Invalidate(&dispatchTestCluster.Spec).MinTimes(1)
	}
	return &Client{factory: factory, helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}
}

// Deadlines for the tests that drive real requests. Under load a TLS
// handshake with a local server can take hundreds of milliseconds, so no case
// may depend on one finishing quickly.
const (
	// answerDeadline only stops a hung test: the server answers these
	// requests.
	answerDeadline = 10 * time.Second
	// noConnDeadline ends a request that never gets a connection. The request
	// is unreached whether the deadline or the failure ends it.
	noConnDeadline = 300 * time.Millisecond
	// stallDeadline ends a request the server read and never answered. It has
	// to outlast connecting and writing the request.
	stallDeadline = 2 * time.Second
)

// testContext returns a context that ends after d, or after answerDeadline
// when d is zero.
func testContext(t *testing.T, d time.Duration) context.Context {
	if d == 0 {
		d = answerDeadline
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

// TestCreateJobCluster_ClassifiesRealRequests drives real requests over TLS
// and HTTP/2: a request is unreached only if it got no connection.
func TestCreateJobCluster_ClassifiesRealRequests(t *testing.T) {
	tests := []struct {
		name       string
		config     func(t *testing.T) *rest.Config
		want       DispatchClass
		invalidate bool
		deadline   time.Duration // answerDeadline when zero
	}{
		{name: "listener accepts TCP but never completes TLS", config: silentTLSListener, want: DispatchUnreached, deadline: noConnDeadline},
		{name: "connection refused", config: refusedConfig, want: DispatchUnreached, deadline: noConnDeadline},
		{name: "DNS failure", config: func(*testing.T) *rest.Config { return &rest.Config{Host: "https://compute-a.invalid"} }, want: DispatchUnreached, deadline: noConnDeadline},
		// The handshake has to finish before it can fail, so this case gets the
		// full deadline.
		{name: "x509 failure invalidates", config: untrustedTLSServer, want: DispatchUnreached, invalidate: true},
		{name: "server reads the full request then stalls", config: stallingServer, want: DispatchAmbiguous, deadline: stallDeadline},
		{name: "already exists", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusConflict, metav1.StatusReasonAlreadyExists, "exists")
		}, want: DispatchAlreadyExists},
		{name: "403", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusForbidden, metav1.StatusReasonForbidden, `admission webhook "vraycluster.kb.io" denied the request`)
		}, want: DispatchPermanent},
		{name: "422", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusUnprocessableEntity, metav1.StatusReasonInvalid, "invalid")
		}, want: DispatchPermanent},
		{name: "401 invalidates", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusUnauthorized, metav1.StatusReasonUnauthorized, "Unauthorized")
		}, want: DispatchRejected, invalidate: true},
		{name: "404", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusNotFound, metav1.StatusReasonNotFound, "namespace not found")
		}, want: DispatchRejected},
		{name: "409 conflict", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusConflict, metav1.StatusReasonConflict, "conflict")
		}, want: DispatchRejected},
		{name: "429", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusTooManyRequests, metav1.StatusReasonTooManyRequests, "slow down")
		}, want: DispatchRejected},
		{name: "503", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, "unavailable")
		}, want: DispatchRejected},
		{name: "webhook 500", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusInternalServerError, metav1.StatusReasonInternalError,
				`Internal error occurred: failed calling webhook "mraycluster.kb.io": failed to call webhook: context deadline exceeded`)
		}, want: DispatchRejected},
		{name: "408", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusRequestTimeout, metav1.StatusReasonTimeout, "timeout")
		}, want: DispatchAmbiguous},
		{name: "504", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusGatewayTimeout, metav1.StatusReasonTimeout, "timeout")
		}, want: DispatchAmbiguous},
		{name: "other 500", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusInternalServerError, metav1.StatusReasonInternalError, "etcd leader changed")
		}, want: DispatchAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := clientFor(t, tt.config(t), tt.invalidate)
			rc := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "ns1"}}

			err := c.CreateJobCluster(testContext(t, tt.deadline), rc, dispatchTestCluster)

			require.Error(t, err)
			assert.Equal(t, tt.want, ClassifyDispatchError(err), "error: %v", err)
		})
	}
}

// TestCreateJobCluster_ConnectBound: a TLS handshake that never completes is
// cut at the connect bound, well before the request's deadline, and the
// attempt is unreached.
func TestCreateJobCluster_ConnectBound(t *testing.T) {
	withConnectBound(t, 200*time.Millisecond)
	c := clientFor(t, silentTLSListener(t), false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	start := time.Now()
	err := c.CreateJobCluster(ctx, &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc"}}, dispatchTestCluster)

	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.ErrorIs(t, err, errConnectTimeout)
	assert.Contains(t, err.Error(), "create ray cluster err:"+errConnectTimeout.Error())
	assert.Equal(t, DispatchUnreached, ClassifyDispatchError(err))
}

// TestCreateJobCluster_ConnectBoundStopsAtConnection: once a connection is
// obtained the bound no longer applies, so a slow answer is not cut by it.
func TestCreateJobCluster_ConnectBoundStopsAtConnection(t *testing.T) {
	// The bound has to outlast the handshake; the answer comes after it.
	const bound = time.Second
	withConnectBound(t, bound)
	server := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(bound + 250*time.Millisecond)
		writeStatus(w, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, "unavailable")
	})
	c := clientFor(t, trustingConfig(server), false)

	err := c.CreateJobCluster(context.Background(), &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc"}}, dispatchTestCluster)

	require.Error(t, err)
	assert.NotErrorIs(t, err, errConnectTimeout)
	assert.Equal(t, DispatchRejected, ClassifyDispatchError(err), "error: %v", err)
}

func TestCreateJobCluster_Deadline(t *testing.T) {
	assert.Equal(t, 20*time.Second, DispatchRequestTimeout)
	assert.Equal(t, 5*time.Second, connectBound)

	var deadline time.Time
	server := h2Server(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"apiVersion":"ray.io/v1","kind":"RayCluster","metadata":{"name":"rc"}}`))
	})
	c := clientFor(t, trustingConfig(server), false)
	helper := c.helper
	c.helper = deadlineRecordingHelper{Helper: helper, deadline: &deadline}

	start := time.Now()
	require.NoError(t, c.CreateJobCluster(context.Background(), &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc"}}, dispatchTestCluster))
	assert.WithinDuration(t, start.Add(DispatchRequestTimeout), deadline, 5*time.Second)
}

// deadlineRecordingHelper records the deadline each create runs under.
type deadlineRecordingHelper struct {
	Helper
	deadline *time.Time
}

func (h deadlineRecordingHelper) CreateResource(ctx context.Context, client rest.Interface, obj runtime.Object, resource, namespace string) error {
	*h.deadline, _ = ctx.Deadline()
	return h.Helper.CreateResource(ctx, client, obj, resource, namespace)
}

func TestCreateJobCluster_Sentinels(t *testing.T) {
	t.Run("client construction", func(t *testing.T) {
		factory := computemocks.NewMockFactory(gomock.NewController(t))
		factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, errors.New("dummy error"))
		c := &Client{factory: factory, helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}

		err := c.CreateJobCluster(context.Background(), &v2pb.RayCluster{}, dispatchTestCluster)

		require.EqualError(t, err, "get client for cluster err:dummy error")
		assert.ErrorIs(t, err, ErrClusterClientUnavailable)
		assert.Equal(t, DispatchUnreached, ClassifyDispatchError(err))
	})
	t.Run("spec mapping", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		factory := computemocks.NewMockFactory(ctrl)
		factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
		mapper := typesmocks.NewMockMapper(ctrl)
		mapper.EXPECT().GetLocalName(gomock.Any()).Return("default", "rc")
		mapper.EXPECT().MapGlobalJobClusterToLocal(gomock.Any(), gomock.Any()).Return(nil, errors.New("no project identity"))
		c := &Client{factory: factory, helper: NewHelper(), mapper: mapper, logger: zap.NewNop()}

		err := c.CreateJobCluster(context.Background(), &v2pb.RayCluster{}, dispatchTestCluster)

		require.EqualError(t, err, "map global to local err:no project identity")
		assert.ErrorIs(t, err, ErrJobSpecMapping)
		assert.Equal(t, DispatchPermanent, ClassifyDispatchError(err))
	})
}

func TestCreateJob_Sentinels(t *testing.T) {
	rayJob := &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns1"}}
	rayCluster := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "ns1"}}

	t.Run("client construction", func(t *testing.T) {
		factory := computemocks.NewMockFactory(gomock.NewController(t))
		factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(nil, errors.New("dummy error"))
		c := &Client{factory: factory, helper: NewHelper(), mapper: permissiveMapper(t), logger: zap.NewNop()}

		err := c.CreateJob(context.Background(), rayJob, rayCluster, dispatchTestCluster)

		require.EqualError(t, err, "get client for cluster err:dummy error")
		assert.ErrorIs(t, err, ErrClusterClientUnavailable)
		assert.Equal(t, DispatchUnreached, ClassifyDispatchError(err))
	})
	t.Run("spec mapping", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		factory := computemocks.NewMockFactory(ctrl)
		factory.EXPECT().GetClientSetForCluster(gomock.Any()).Return(&compute.ClientSet{}, nil)
		mapper := typesmocks.NewMockMapper(ctrl)
		mapper.EXPECT().MapGlobalJobToLocal(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("no head"))
		c := &Client{factory: factory, helper: NewHelper(), mapper: mapper, logger: zap.NewNop()}

		err := c.CreateJob(context.Background(), rayJob, rayCluster, dispatchTestCluster)

		require.EqualError(t, err, "map global to local err:no head")
		assert.ErrorIs(t, err, ErrJobSpecMapping)
		assert.Equal(t, DispatchPermanent, ClassifyDispatchError(err))
	})
}

// TestCreateJob_OwnerReferenceGetClassifiesLikeACreate: the GET of the
// compute-side RayCluster before the create classifies the same way, and
// both run under the connect bound.
func TestCreateJob_OwnerReferenceGetClassifiesLikeACreate(t *testing.T) {
	rayJob := &v2pb.RayJob{ObjectMeta: metav1.ObjectMeta{Name: "rj", Namespace: "ns1"}}
	rayCluster := &v2pb.RayCluster{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "ns1"}}
	remoteCluster := `{"apiVersion":"ray.io/v1","kind":"RayCluster","metadata":{"name":"rc","namespace":"default","uid":"remote-uid"}}`
	answerGet := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet {
			return false
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(remoteCluster))
		return true
	}

	tests := []struct {
		name        string
		config      func(t *testing.T) *rest.Config
		want        DispatchClass
		wantPrefix  string
		wantConnect bool
		deadline    time.Duration // answerDeadline when zero
	}{
		{name: "GET gets no connection", config: silentTLSListener, want: DispatchUnreached, wantPrefix: "get remote ray cluster for owner ref: ", wantConnect: true},
		{name: "GET answered 404", config: func(t *testing.T) *rest.Config {
			return statusServer(t, http.StatusNotFound, metav1.StatusReasonNotFound, "not found")
		}, want: DispatchRejected, wantPrefix: "get remote ray cluster for owner ref: "},
		{name: "GET read then stalls", config: stallingServer, want: DispatchAmbiguous, wantPrefix: "get remote ray cluster for owner ref: ", deadline: stallDeadline},
		{name: "GET succeeds, create answered 503", config: func(t *testing.T) *rest.Config {
			return trustingConfig(h2Server(t, func(w http.ResponseWriter, r *http.Request) {
				if answerGet(w, r) {
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				writeStatus(w, http.StatusServiceUnavailable, metav1.StatusReasonServiceUnavailable, "unavailable")
			}))
		}, want: DispatchRejected, wantPrefix: "create ray job err:"},
		{name: "GET succeeds, create read then stalls", config: func(t *testing.T) *rest.Config {
			return trustingConfig(stallingH2Server(t, answerGet))
		}, want: DispatchAmbiguous, wantPrefix: "create ray job err:", deadline: stallDeadline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantConnect {
				// Only a case that never connects may cut the handshake short.
				withConnectBound(t, 100*time.Millisecond)
			}
			c := clientFor(t, tt.config(t), false)

			err := c.CreateJob(testContext(t, tt.deadline), rayJob, rayCluster, dispatchTestCluster)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantPrefix)
			assert.Equal(t, tt.wantConnect, errors.Is(err, errConnectTimeout), "error: %v", err)
			assert.Equal(t, tt.want, ClassifyDispatchError(err), "error: %v", err)
		})
	}
}
