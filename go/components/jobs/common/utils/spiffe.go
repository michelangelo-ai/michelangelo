package utils

import (
	"fmt"
	"os/user"

	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/constants"
)

const (
	defaultSpiffeID = constants.GenericSpiffeAnnotationValue
	spiffeIDFormat  = "k8s-batch/uid/%s"
)

// SpiffeIDProvider is an interface for fetching SPIFFE IDs.
type SpiffeIDProvider interface {
	GetSpiffeID(ldap string) string
	GetUserID(ldap string) string
}

// UserLookupFunc allows us to inject a custom lookup function for testing.
type UserLookupFunc func(username string) (*user.User, error)

// DefaultSpiffeIDProvider is the default implementation of SpiffeIDProvider.
type DefaultSpiffeIDProvider struct {
	lookupFunc UserLookupFunc
}

// NewDefaultSpiffeIDProvider creates a new provider with the default user lookup function.
func NewDefaultSpiffeIDProvider() *DefaultSpiffeIDProvider {
	return &DefaultSpiffeIDProvider{
		lookupFunc: user.Lookup,
	}
}

// GetSpiffeID fetches the SPIFFE ID based on LDAP.
func (p *DefaultSpiffeIDProvider) GetSpiffeID(ldap string) string {
	userDetails, err := p.lookupFunc(ldap)
	if err != nil {
		return defaultSpiffeID
	}
	return fmt.Sprintf(spiffeIDFormat, userDetails.Uid)
}

// GetUserID fetches just the user ID based on LDAP.
func (p *DefaultSpiffeIDProvider) GetUserID(ldap string) string {
	userDetails, err := p.lookupFunc(ldap)
	if err != nil {
		return ""
	}
	return userDetails.Uid
}
