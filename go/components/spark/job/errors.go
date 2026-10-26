package job

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
)

// isTransientError reports whether err is a failure that the same request may
// well not hit again: the API server, or something in front of it, timed out,
// was overloaded or answered with something that is not a Kubernetes status;
// the call's deadline passed or its context was cancelled; the connection was
// refused, reset, cut short or timed out; or the request never reached the
// server at all, because dialing it failed (no route to its host or network,
// and the like) or its name did not resolve. Errors are matched anywhere in a
// %w chain, which the federated client keeps intact for member-cluster and
// client-factory errors.
//
// It is this package's one transient-error predicate. launch uses it to tell a
// SparkApplication creation worth retrying from one that would fail the same
// way again, and the watch's status writes retry on
// apiErrors.IsConflict(err) || isTransientError(err).
//
// Everything else is not transient, which for a creation means terminal:
// validation and admission rejections, authorization failures, a name already
// taken by an application that is not the job's own
// (jobsclient.ErrSparkApplicationConflict), a SparkJob the mapper cannot
// project, and a failure on a connection that was made, such as a certificate
// the client does not trust. A failed dial or name lookup counts as transient
// even when the member cluster's endpoint is simply wrong, since the error
// cannot tell that from a cluster that is unreachable for now: the job stays
// unlaunched and its creation is retried on the controller's backoff, rather
// than every job scheduled to that cluster failing.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case apiErrors.IsServerTimeout(err),
		apiErrors.IsTimeout(err),
		apiErrors.IsTooManyRequests(err),
		apiErrors.IsServiceUnavailable(err),
		apiErrors.IsInternalError(err),
		apiErrors.IsUnexpectedServerError(err):
		return true
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		// context.DeadlineExceeded is also a net.Error that timed out, so the
		// last check would catch it too; it is named here for the reader.
		return true
	case utilnet.IsConnectionRefused(err), utilnet.IsConnectionReset(err), utilnet.IsProbableEOF(err):
		return true
	case isDialError(err):
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isRemoteTimeout reports whether err shows that a member cluster did not
// answer a call in time: the call's deadline passed (context.DeadlineExceeded,
// which remoteCallTimeout sets), or the connection timed out on the way (a
// net.Error whose Timeout() is true: a dial, a read waiting for the response,
// or a lookup of the cluster's name that timed out). Errors are matched
// anywhere in a %w chain. The deadline is matched on its own as well because a
// *url.Error, itself a net.Error, reports only whether the error directly
// beneath it timed out, so a deadline wrapped once more below it would be
// missed by the second check alone.
//
// These are the failures that start a hold on their cluster (holdCluster):
// each of them cost a worker up to remoteCallTimeout. isTransientError matches
// all of them too. A failure that came back fast does not count: a refused or
// reset connection, a dial with no route, a name that does not resolve, and any
// answer from the API server or a proxy in front of it, a 503, a 429 or a
// timeout the server reports itself (a Timeout or ServerTimeout status)
// included. Nor does a cancelled context, which is the controller shutting
// down rather than the cluster failing to answer.
func isRemoteTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isDialError reports whether a *net.OpError for a dial is anywhere in err's
// chain, which means the request failed before it reached the server. The
// search goes on below an OpError for another operation, because a request
// sent through a proxy reports a failed dial wrapped in an OpError of the
// proxy's own ("proxyconnect").
func isDialError(err error) bool {
	var opErr *net.OpError
	for errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return true
		}
		err = opErr.Err
	}
	return false
}

// isSparkApplicationAbsent reports whether err, from a call to a member cluster
// about the job's SparkApplication, shows that the application is not there: a
// NotFound status from the cluster's API server, matched anywhere in a %w
// chain, which the federated client keeps intact.
//
// A 404 whose body is not a Kubernetes Status does not count. The API server
// answers a request for an object it does not have, in an API it serves, with
// a Status, so a 404 without one comes from one of two places, and nothing in
// it reliably tells which:
//   - A proxy, an ingress or a load balancer in front of the API server, one
//     answering for a route it cannot serve right now, during a reload or
//     after a misroute, say. The application may well be running.
//   - The member cluster's own API server, when it does not serve the
//     SparkApplication API at all: its last handler answers a plain-text "404
//     page not found" for a path no API it serves covers. That happens once
//     the Spark Operator's CRD is removed, which takes every application with
//     it, or if it was never installed.
//
// client-go reports either as NotFound all the same, marked as an unexpected
// server response, which isTransientError counts as transient. Such a 404 is
// a failure like any other, never proof that the application is gone: a probe
// that meets one records nothing and holds the cluster (holdCluster), a
// termination that meets one is retried rather than recorded as done, and so
// is a deletion, until handleDeletion gives up on it an hour after the job's
// deletion (sparkApplicationCleanupGiveUpAfter).
func isSparkApplicationAbsent(err error) bool {
	return apiErrors.IsNotFound(err) && !apiErrors.IsUnexpectedServerError(err)
}

// sanitizeConditionMessage turns an error from a member-cluster call into text
// for a condition on the SparkJob, which the job's authors can read. The full
// error belongs in the log instead: a client-go transport error carries
// the request's method and URL, and with them the member cluster's internal
// API endpoint.
//
//   - A Kubernetes API status anywhere in the chain: its message, which is what
//     the member cluster's API server, or one of its admission webhooks, said.
//   - Otherwise a *url.Error anywhere in the chain: the error underneath it,
//     introduced by the cluster's name rather than the request's URL, and
//     without the addresses of a *net.OpError (withoutAddresses).
//   - Otherwise the error as it is: the control plane's own errors, such as
//     a name conflict with an application that is not the job's own, name no
//     endpoint.
//
// A status with an empty message falls through to the next rule, so the
// condition is never left blank.
func sanitizeConditionMessage(err error, clusterName string) string {
	var status apiErrors.APIStatus
	if errors.As(err, &status) && status.Status().Message != "" {
		return status.Status().Message
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Sprintf("request to cluster %q failed: %s", clusterName, withoutAddresses(urlErr.Err))
	}
	return err.Error()
}

// withoutAddresses renders err, the error beneath a *url.Error, leaving out the
// addresses a *net.OpError names: the member cluster's API endpoint and, once a
// connection was made, its local end as well, whose port changes from one
// attempt to the next. The client puts the dialer's or the reader's OpError
// right beneath the *url.Error, and an OpError right beneath another, as a
// proxy's own OpError wraps the failed dial to the proxy, loses its addresses
// too. Any other error is rendered as it is.
func withoutAddresses(err error) string {
	opErr, ok := err.(*net.OpError)
	if !ok {
		return err.Error()
	}
	text := opErr.Op
	if opErr.Net != "" {
		text += " " + opErr.Net
	}
	return text + ": " + withoutAddresses(opErr.Err)
}
