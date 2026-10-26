package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/michelangelo-ai/michelangelo/go/components/jobs/client"
	"github.com/michelangelo-ai/michelangelo/go/components/jobs/common/constants"
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// failedProbeStatus returns the status to store when probing a cluster failed,
// which is never Ready: the status the probe returned, or an Offline one
// carrying the error when the probe returned none (its client could not be
// built) or one that claims Ready.
func failedProbeStatus(status *v2pb.ClusterStatus, err error, now metav1.Time) *v2pb.ClusterStatus {
	if len(status.GetStatusConditions()) > 0 && !isClusterReady(status) {
		return status
	}
	return &v2pb.ClusterStatus{
		StatusConditions: []*apipb.Condition{
			{
				Type:                 constants.ClusterOffline,
				Status:               apipb.CONDITION_STATUS_TRUE,
				Reason:               client.ClusterNotReachableReason,
				Message:              fmt.Sprintf("%s: %v", client.ClusterNotReachableMsg, err),
				LastUpdatedTimestamp: now.Unix(),
			},
		},
	}
}

// shouldInvalidateClient reports whether a probe error means the cluster no
// longer accepts the cached client's credentials: the token was rejected (401)
// or the server certificate failed verification. Rebuilding the client re-reads
// the token and CA from their Secrets, which is how a token or CA rotated in
// place takes effect.
func shouldInvalidateClient(err error) bool {
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

// clientNeedsRebuild reports whether a spec change touches what the cluster's
// client is built from: the API server's host and port, or the Secrets holding
// its token and CA.
func clientNeedsRebuild(oldSpec, newSpec *v2pb.ClusterSpec) bool {
	oldRest, newRest := oldSpec.GetKubernetes().GetRest(), newSpec.GetKubernetes().GetRest()
	return oldRest.GetHost() != newRest.GetHost() ||
		oldRest.GetPort() != newRest.GetPort() ||
		oldRest.GetTokenTag() != newRest.GetTokenTag() ||
		oldRest.GetCaDataTag() != newRest.GetCaDataTag()
}
