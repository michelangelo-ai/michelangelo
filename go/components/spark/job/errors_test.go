package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	jobsclient "github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
)

// sparkApplicationsResource is the resource the member clusters' API servers
// name in their errors about a projected SparkApplication.
var sparkApplicationsResource = schema.GroupResource{Group: "sparkoperator.k8s.io", Resource: "sparkapplications"}

// memberAPIURL stands in for a member cluster's internal API endpoint, which a
// client-go transport error carries and a condition message must not.
const memberAPIURL = "https://10.20.30.40:6443/apis/sparkoperator.k8s.io/v1beta2/namespaces/default/sparkapplications"

// transportError is what client-go returns when a request to memberAPIURL fails
// below HTTP: a *url.Error around the dialer's or the reader's error.
func transportError(err error) error {
	return &url.Error{Op: "Post", URL: memberAPIURL, Err: err}
}

// unstructuredNotFound is what client-go returns for a 404 from a member
// cluster whose body is not a Kubernetes status, such as the page a proxy or an
// ingress in front of the API server serves for a route it cannot serve: an
// error that is NotFound and is marked as an unexpected server response. The
// API server's own plain-text 404 for an API it does not serve
// (apiNotServedNotFound) comes out the same way.
func unstructuredNotFound() error {
	return apiErrors.NewGenericServerResponse(http.StatusNotFound, "GET", sparkApplicationsResource,
		sparkJobName, "<html>404 page not found</html>", 0, true)
}

// apiNotServedNotFound is what client-go returns when a member cluster's API
// server does not serve the SparkApplication API, its CRD removed or never
// installed: the server's last handler answers the path with a plain-text "404
// page not found", which client-go reports as a NotFound marked as an
// unexpected server response, just like unstructuredNotFound.
func apiNotServedNotFound() error {
	return apiErrors.NewGenericServerResponse(http.StatusNotFound, "GET", sparkApplicationsResource,
		sparkJobName, "404 page not found", 0, true)
}

// opaqueError wraps an error under a message of its own, so a row can reach an
// error's type through the chain without its text: the probable-EOF check
// matches on text, and some rows must be classified by type alone.
type opaqueError struct{ err error }

func (e opaqueError) Error() string { return "request failed" }
func (e opaqueError) Unwrap() error { return e.err }

// dialError is the error a dial fails with when connect(2) returns errno: the
// request never left the client.
func dialError(errno syscall.Errno) error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", errno)}
}

// dnsNotFoundError is a lookup of the member cluster's API host that found no
// such name.
func dnsNotFoundError() error {
	return &net.DNSError{Err: "no such host", Name: "api.member.internal", IsNotFound: true}
}

// dnsTemporaryError is a lookup of the member cluster's API host that the name
// server could not answer for now.
func dnsTemporaryError() error {
	return &net.DNSError{Err: "server misbehaving", Name: "api.member.internal", Server: "10.0.0.10:53", IsTemporary: true}
}

// TestIsTransientError pins the classifier that decides whether a failed
// creation is retried or recorded as the job's outcome, and whether the watch
// retries a status write. Every rule has a row that only it matches, so
// dropping a rule fails a row of its own; the exception is the deadline rule,
// whose row the net-timeout rule matches too, since a deadline that passed is
// also a net.Error that timed out. A refused connection is also a failed dial,
// so the realistic "connection refused" row is matched by two rules, and the
// refused rule's own row reaches the errno through opaqueError, with no dial in
// the chain. Each row also appears wrapped the way the federated client wraps
// it, which the classifier has to see through.
func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// Transient: the same request may well succeed next time.
		{name: "server timeout", err: apiErrors.NewServerTimeout(sparkApplicationsResource, "create", 1), want: true},
		{name: "timeout", err: apiErrors.NewTimeoutError("request did not complete within the allotted timeout", 1), want: true},
		{name: "too many requests", err: apiErrors.NewTooManyRequests("the server has received too many requests", 1), want: true},
		{name: "service unavailable", err: apiErrors.NewServiceUnavailable("etcdserver: leader changed"), want: true},
		{name: "internal error", err: apiErrors.NewInternalError(errors.New("failed calling webhook")), want: true},
		{
			// A 400 is not transient in itself; what makes this one transient is
			// that the body was not a Kubernetes status, which is what a proxy or
			// a load balancer in front of the API server answers with.
			name: "response that is not a Kubernetes status",
			err: apiErrors.NewGenericServerResponse(http.StatusBadRequest, "POST", sparkApplicationsResource,
				sparkJobName, "<html>bad gateway</html>", 0, true),
			want: true,
		},
		{name: "deadline exceeded", err: transportError(context.DeadlineExceeded), want: true},
		{name: "cancelled", err: context.Canceled, want: true},
		{name: "connection refused", err: transportError(dialError(syscall.ECONNREFUSED)), want: true},
		{
			name: "connection refused, with no dial in the chain",
			err:  opaqueError{err: os.NewSyscallError("connect", syscall.ECONNREFUSED)},
			want: true,
		},
		{name: "connection reset", err: opaqueError{err: os.NewSyscallError("read", syscall.ECONNRESET)}, want: true},
		{name: "probable EOF", err: transportError(io.EOF), want: true},
		{
			name: "network timeout",
			err:  transportError(&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}),
			want: true,
		},
		// The request never reached the server: a dial that failed, or a name
		// that did not resolve. client-go returns them inside a *url.Error.
		{name: "dial: no route to host", err: dialError(syscall.EHOSTUNREACH), want: true},
		{name: "dial: no route to host, inside a url.Error", err: transportError(dialError(syscall.EHOSTUNREACH)), want: true},
		{name: "dial: network is unreachable", err: dialError(syscall.ENETUNREACH), want: true},
		{name: "dial: network is unreachable, inside a url.Error", err: transportError(dialError(syscall.ENETUNREACH)), want: true},
		{
			// A proxy reports a failed dial wrapped in an OpError of its own.
			name: "dial through a proxy: no route to host",
			err:  transportError(&net.OpError{Op: "proxyconnect", Net: "tcp", Err: dialError(syscall.EHOSTUNREACH)}),
			want: true,
		},
		{name: "DNS: no such host", err: dnsNotFoundError(), want: true},
		{name: "DNS: no such host, inside a url.Error", err: transportError(dnsNotFoundError()), want: true},
		{name: "DNS: temporary failure", err: dnsTemporaryError(), want: true},
		{name: "DNS: temporary failure, inside a url.Error", err: transportError(dnsTemporaryError()), want: true},

		// Terminal: the same request would fail the same way.
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("quota exceeded on member cluster"), want: false},
		{
			name: "invalid",
			err: apiErrors.NewInvalid(schema.GroupKind{Group: "sparkoperator.k8s.io", Kind: "SparkApplication"}, sparkJobName,
				field.ErrorList{field.Required(field.NewPath("spec", "mainApplicationFile"), "")}),
			want: false,
		},
		{
			name: "forbidden by an admission webhook",
			err:  apiErrors.NewForbidden(sparkApplicationsResource, sparkJobName, errors.New("admission webhook denied the request")),
			want: false,
		},
		{name: "bad request", err: apiErrors.NewBadRequest("spec.driver.cores: must be positive"), want: false},
		{
			name: "a 400 the server explained",
			err: apiErrors.NewGenericServerResponse(http.StatusBadRequest, "POST", sparkApplicationsResource,
				sparkJobName, "spec.driver.cores: must be positive", 0, false),
			want: false,
		},
		{name: "conflict", err: apiErrors.NewConflict(sparkApplicationsResource, sparkJobName, errors.New("object was modified")), want: false},
		{name: "already exists", err: apiErrors.NewAlreadyExists(sparkApplicationsResource, sparkJobName), want: false},
		{
			name: "name taken by another job's application",
			err: fmt.Errorf("%w: default/%s on cluster %q belongs to job UID %q",
				jobsclient.ErrSparkApplicationConflict, sparkJobName, testClusterName, "another-job-uid"),
			want: false,
		},
		{name: "mapper failure", err: fmt.Errorf("map global to local err:%w", errors.New("unknown spark version")), want: false},
		{
			// The connection was made: the server is there, and the client does
			// not trust it.
			name: "certificate the client does not trust",
			err:  transportError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isTransientError(tc.err))
		})
		if tc.err == nil {
			continue
		}
		t.Run(tc.name+", wrapped by the federated client", func(t *testing.T) {
			assert.Equal(t, tc.want, isTransientError(fmt.Errorf("create spark application err:%w", tc.err)))
		})
	}
}

// TestIsRemoteTimeout pins which failures show that a member cluster did not
// answer in time, the ones that hold the cluster: a deadline that passed and a
// connection that timed out, and nothing that came back fast, an answer from
// the API server included. The deadline that sits below a *url.Error with
// something in between is matched only by the deadline rule, and the read and
// dial timeouts and the lookup that timed out only by the net-timeout rule, so
// dropping either rule fails a row of its own. Each row also appears wrapped
// the way the federated client wraps it.
func TestIsRemoteTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// The cluster did not answer in time.
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: true},
		{name: "deadline exceeded, inside a url.Error", err: transportError(context.DeadlineExceeded), want: true},
		{
			// A url.Error reports only whether the error right beneath it timed
			// out, which this wrapped deadline does not say of itself.
			name: "deadline exceeded, wrapped once more inside a url.Error",
			err:  transportError(fmt.Errorf("waiting for the response: %w", context.DeadlineExceeded)),
			want: true,
		},
		{
			name: "read timed out",
			err:  transportError(&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}),
			want: true,
		},
		{name: "dial timed out", err: transportError(dialError(syscall.ETIMEDOUT)), want: true},
		{
			name: "DNS: lookup timed out",
			err:  transportError(&net.DNSError{Err: "i/o timeout", Name: "api.member.internal", IsTimeout: true}),
			want: true,
		},

		// The failure came back fast, or is not the cluster's at all.
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("member cluster unreachable"), want: false},
		{name: "cancelled", err: context.Canceled, want: false},
		{name: "cancelled, inside a url.Error", err: transportError(context.Canceled), want: false},
		{name: "connection refused", err: transportError(dialError(syscall.ECONNREFUSED)), want: false},
		{
			name: "connection reset",
			err:  transportError(&net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}),
			want: false,
		},
		{name: "probable EOF", err: transportError(io.EOF), want: false},
		{name: "dial: no route to host", err: transportError(dialError(syscall.EHOSTUNREACH)), want: false},
		{name: "DNS: no such host", err: transportError(dnsNotFoundError()), want: false},
		{name: "DNS: temporary failure", err: transportError(dnsTemporaryError()), want: false},
		{name: "service unavailable", err: apiErrors.NewServiceUnavailable("etcdserver: leader changed"), want: false},
		{name: "too many requests", err: apiErrors.NewTooManyRequests("the server has received too many requests", 1), want: false},
		{
			// The API server answered: it gave up on the request itself.
			name: "timeout status",
			err:  apiErrors.NewTimeoutError("request did not complete within the allotted timeout", 1),
			want: false,
		},
		{name: "server timeout status", err: apiErrors.NewServerTimeout(sparkApplicationsResource, "create", 1), want: false},
		{name: "a 404 whose body is not a Kubernetes status", err: unstructuredNotFound(), want: false},
		{
			name: "certificate the client does not trust",
			err:  transportError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority")),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRemoteTimeout(tc.err))
		})
		if tc.err == nil {
			continue
		}
		t.Run(tc.name+", wrapped by the federated client", func(t *testing.T) {
			assert.Equal(t, tc.want, isRemoteTimeout(fmt.Errorf("create spark application err:%w", tc.err)))
		})
	}
}

// TestIsSparkApplicationAbsent pins what proves that a SparkApplication is
// gone from its member cluster: a NotFound status from the cluster's API
// server, and nothing else. A 404 whose body is not a Kubernetes status is
// NotFound to client-go too, but a proxy's cannot be told from that of an API
// server that does not serve the SparkApplication API, so neither is proof
// that the application is gone. Each row also appears wrapped the way the
// federated client wraps it, which the check has to see through.
func TestIsSparkApplicationAbsent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "not found", err: apiErrors.NewNotFound(sparkApplicationsResource, sparkJobName), want: true},
		{
			// It is the mark of an unexpected server response that rules this
			// one out: client-go reads it as NotFound like the row above.
			name: "a 404 whose body is not a Kubernetes status",
			err:  unstructuredNotFound(),
			want: false,
		},
		{
			// The application went with the CRD, but this 404 looks like the
			// proxy's above.
			name: "the API server's plain-text 404 for an API it does not serve",
			err:  apiNotServedNotFound(),
			want: false,
		},
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("member cluster unreachable"), want: false},
		{name: "deadline exceeded", err: transportError(context.DeadlineExceeded), want: false},
		{name: "service unavailable", err: apiErrors.NewServiceUnavailable("etcdserver: leader changed"), want: false},
		{
			name: "forbidden",
			err: apiErrors.NewForbidden(sparkApplicationsResource, sparkJobName,
				errors.New("the control plane's service account cannot get sparkapplications")),
			want: false,
		},
		{
			// Nothing between the controller and a member cluster speaks gRPC,
			// so a NotFound in a gRPC status is not a member cluster's answer.
			name: "not found in a gRPC status",
			err:  grpcstatus.Error(codes.NotFound, "resource not found"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSparkApplicationAbsent(tc.err))
		})
		if tc.err == nil {
			continue
		}
		t.Run(tc.name+", wrapped by the federated client", func(t *testing.T) {
			wrapped := fmt.Errorf("get spark application %q status: %w", sparkJobName, tc.err)
			assert.Equal(t, tc.want, isSparkApplicationAbsent(wrapped))
		})
	}
}

// TestSanitizeConditionMessage pins what a failed creation records on the
// SparkJob, where the job's authors read it: what the API server said
// when it said something, and never the member cluster's endpoint.
func TestSanitizeConditionMessage(t *testing.T) {
	var (
		memberAPIAddr = &net.TCPAddr{IP: net.ParseIP("10.20.30.40"), Port: 6443}
		localAddr     = &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 54321}
		proxyAddr     = &net.TCPAddr{IP: net.ParseIP("10.9.9.9"), Port: 3128}
	)
	forbidden := apiErrors.NewForbidden(sparkApplicationsResource, sparkJobName,
		errors.New(`admission webhook "validate.sparkoperator.k8s.io" denied the request: spec.driver.cores must be positive`))
	conflict := fmt.Errorf("%w: default/%s on cluster %q belongs to job UID %q",
		jobsclient.ErrSparkApplicationConflict, sparkJobName, testClusterName, "another-job-uid")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an API status gives its message, without the client's wrapping",
			err:  fmt.Errorf("create spark application err:%w", forbidden),
			want: forbidden.Status().Message,
		},
		{
			name: "a transport error gives the error underneath, introduced by the cluster's name",
			err: fmt.Errorf("create spark application err:%w",
				transportError(errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"))),
			want: `request to cluster "member-cluster-1" failed: tls: failed to verify certificate: x509: certificate signed by unknown authority`,
		},
		{
			name: "a refused connection gives the error underneath without the member cluster's address",
			err: fmt.Errorf("create spark application err:%w", transportError(&net.OpError{
				Op: "dial", Net: "tcp", Addr: memberAPIAddr, Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
			})),
			want: `request to cluster "member-cluster-1" failed: dial tcp: connect: connection refused`,
		},
		{
			// The local end's port changes from one attempt to the next, so a
			// message that kept it would differ every time.
			name: "a reset connection gives the error underneath without either end's address",
			err: transportError(&net.OpError{
				Op: "read", Net: "tcp", Source: localAddr, Addr: memberAPIAddr, Err: os.NewSyscallError("read", syscall.ECONNRESET),
			}),
			want: `request to cluster "member-cluster-1" failed: read tcp: read: connection reset by peer`,
		},
		{
			name: "a failed dial through a proxy loses the addresses at both levels",
			err: transportError(&net.OpError{Op: "proxyconnect", Net: "tcp", Err: &net.OpError{
				Op: "dial", Net: "tcp", Addr: proxyAddr, Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
			}}),
			want: `request to cluster "member-cluster-1" failed: proxyconnect tcp: dial tcp: connect: connection refused`,
		},
		{
			name: "the control plane's own error is kept as it is",
			err:  conflict,
			want: conflict.Error(),
		},
		{
			name: "a plain error is kept as it is",
			err:  errors.New("quota exceeded on member cluster"),
			want: "quota exceeded on member cluster",
		},
		{
			name: "an API status with no message does not blank the condition",
			err: fmt.Errorf("create spark application err:%w",
				&apiErrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: http.StatusUnprocessableEntity}}),
			want: "create spark application err:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeConditionMessage(tc.err, testClusterName)

			assert.Equal(t, tc.want, got)
			assert.NotContains(t, got, "10.20.30.40", "the member cluster's endpoint must not reach the condition")
		})
	}
}
