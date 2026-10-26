package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// DispatchRequestTimeout bounds one create on a compute cluster (RFC 0f). It
// is above the API server's 10 s default webhook timeout, so a webhook that
// times out is answered by the API server first.
const DispatchRequestTimeout = 20 * time.Second

// connectBound bounds how long a request waits for a connection to the
// compute cluster. It is a var so that tests can shorten it.
var connectBound = 5 * time.Second

var (
	// ErrClusterClientUnavailable marks errors building the client for a
	// compute cluster (factory or Secret errors): no request was sent.
	ErrClusterClientUnavailable = errors.New("cluster client unavailable")
	// ErrJobSpecMapping marks errors mapping a global job to its compute-side
	// object: the job's spec cannot be dispatched.
	ErrJobSpecMapping = errors.New("job spec mapping failed")

	// errConnectTimeout is the cause a request is canceled with when it got
	// no connection within connectBound.
	errConnectTimeout = errors.New("no connection to the compute cluster within the connect bound")
)

// DispatchClass is the class of a create's outcome on a compute cluster,
// which decides between retrying, re-placing and failing the job (RFC 0f).
type DispatchClass int

const (
	// DispatchSuccess is a create that succeeded.
	DispatchSuccess DispatchClass = iota
	// DispatchAlreadyExists is a create that found an object of the same
	// name. It succeeds only if that object is the job's, which the
	// dispatcher checks with ResolveAlreadyExists.
	DispatchAlreadyExists
	// DispatchPermanent is a create the target refused for good: an invalid
	// or forbidden request, or a spec that cannot be mapped.
	DispatchPermanent
	// DispatchUnreached is a create that provably never reached the server:
	// it got no connection, or writing the request failed.
	DispatchUnreached
	// DispatchRejected is a create the server answered without creating
	// anything.
	DispatchRejected
	// DispatchAmbiguous is a create that may have reached the server and
	// failed without a definite answer: the object may exist.
	DispatchAmbiguous
)

// String returns the class as used in metric labels.
func (c DispatchClass) String() string {
	switch c {
	case DispatchSuccess:
		return "success"
	case DispatchAlreadyExists:
		return "already_exists"
	case DispatchPermanent:
		return "permanent"
	case DispatchUnreached:
		return "unreached"
	case DispatchRejected:
		return "rejected"
	case DispatchAmbiguous:
		return "ambiguous"
	default:
		return "unknown"
	}
}

// ClassifyDispatchError classifies the error of a create on a compute
// cluster. CreateJobCluster's and CreateJob's errors record what their
// request saw of its connection; any other error counts as having reached
// the server, which is the conservative reading.
func ClassifyDispatchError(err error) DispatchClass {
	trace := requestTrace{gotConn: true}
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		trace = reqErr.trace
	}
	return classifyDispatchError(err, trace)
}

// classifyDispatchError applies RFC 0f's table top to bottom; the first
// match wins. An error that carries a response is classified by its status
// and is never unreached. Without a response, a request is unreached only
// when it got no connection or failed to write: over HTTP/2 a request can be
// on the wire before the write is reported, so anything else is ambiguous.
func classifyDispatchError(err error, trace requestTrace) DispatchClass {
	var apiStatus apierrors.APIStatus
	switch {
	case err == nil:
		return DispatchSuccess
	case apierrors.IsAlreadyExists(err):
		return DispatchAlreadyExists
	case errors.Is(err, ErrJobSpecMapping):
		return DispatchPermanent
	case errors.Is(err, ErrClusterClientUnavailable):
		return DispatchUnreached
	case errors.As(err, &apiStatus):
		switch {
		case apierrors.IsBadRequest(err) || apierrors.IsInvalid(err) || apierrors.IsForbidden(err):
			return DispatchPermanent
		case isRejectedStatus(apiStatus):
			return DispatchRejected
		default:
			return DispatchAmbiguous
		}
	case !trace.gotConn || trace.writeFailed:
		return DispatchUnreached
	default:
		return DispatchAmbiguous
	}
}

// isRejectedStatus reports a server answer that created nothing: any 4xx but
// 408, a 503, or a 500 from a webhook the API server failed to call.
func isRejectedStatus(apiStatus apierrors.APIStatus) bool {
	status := apiStatus.Status()
	switch code := status.Code; {
	case code == http.StatusServiceUnavailable:
		return true
	case code == http.StatusInternalServerError:
		return strings.Contains(status.Message, "failed calling webhook")
	case code >= 400 && code < 500:
		return code != http.StatusRequestTimeout
	default:
		return false
	}
}

// isCredentialError reports an error that a rebuilt client may fix: a 401 or
// a TLS verification error (mirrors the cluster health probe, PR3).
func isCredentialError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsUnauthorized(err) {
		return true
	}
	var verificationErr *tls.CertificateVerificationError
	var unknownAuthorityErr x509.UnknownAuthorityError
	var certificateInvalidErr x509.CertificateInvalidError
	var hostnameErr x509.HostnameError
	return errors.As(err, &verificationErr) ||
		errors.As(err, &unknownAuthorityErr) ||
		errors.As(err, &certificateInvalidErr) ||
		errors.As(err, &hostnameErr)
}

// requestTrace is what a traced request saw of its connection, over all of
// client-go's tries.
type requestTrace struct {
	// gotConn is set once any try obtained a connection.
	gotConn bool
	// writeFailed is set once writing a try's request failed.
	writeFailed bool
}

// requestError is the error of a traced request, with its trace.
type requestError struct {
	err   error
	trace requestTrace
}

func (e *requestError) Error() string { return e.err.Error() }
func (e *requestError) Unwrap() error { return e.err }

// tracedRequest runs request under the connect bound, tracing its
// connection for ClassifyDispatchError. The bound starts at the first
// connection lookup and stops once a connection is obtained; if it expires
// first, request's context is canceled with errConnectTimeout.
func tracedRequest(ctx context.Context, request func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	var gotConn, writeFailed atomic.Bool
	var armOnce sync.Once
	var connectTimer atomic.Pointer[time.Timer]
	stopConnectTimer := func() {
		if timer := connectTimer.Load(); timer != nil {
			timer.Stop()
		}
	}
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GetConn: func(string) {
			armOnce.Do(func() {
				connectTimer.Store(time.AfterFunc(connectBound, func() {
					if !gotConn.Load() {
						cancel(errConnectTimeout)
					}
				}))
			})
		},
		GotConn: func(httptrace.GotConnInfo) {
			gotConn.Store(true)
			stopConnectTimer()
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err != nil {
				writeFailed.Store(true)
			}
		},
	})

	err := request(ctx)
	stopConnectTimer()
	if err == nil {
		return nil
	}
	if errors.Is(context.Cause(ctx), errConnectTimeout) {
		err = fmt.Errorf("%w: %w", errConnectTimeout, err)
	}
	return &requestError{err: err, trace: requestTrace{gotConn: gotConn.Load(), writeFailed: writeFailed.Load()}}
}

// markedError adds a sentinel to an error for errors.Is, keeping its message.
type markedError struct {
	err      error
	sentinel error
}

func (e *markedError) Error() string   { return e.err.Error() }
func (e *markedError) Unwrap() []error { return []error{e.err, e.sentinel} }

func markError(err, sentinel error) error {
	return &markedError{err: err, sentinel: sentinel}
}
