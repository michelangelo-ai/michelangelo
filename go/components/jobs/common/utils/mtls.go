package utils

import (
	"context"
	"time"

	"code.uber.internal/rt/flipr-client-go.git/flipr"
	"code.uber.internal/uberai/michelangelo/controllermgr/pkg/controllers/jobs/common/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v2beta1pb "michelangelo/api/v2beta1"
)

var (
	_mtlsTimeout = 20 * time.Second
)

// MTLSHandlerImpl is responsible for determining if mTLS should be enabled
type MTLSHandlerImpl struct {
	apiHandler              api.Handler
	fliprClient             flipr.FliprClient
	fliprConstraintsBuilder types.FliprConstraintsBuilder
}

var _ types.MTLSHandler = MTLSHandlerImpl{}

// NewMTLSHandler creates a new MTLSHandler instance
func NewMTLSHandler(apiHandler api.Handler, fliprClient flipr.FliprClient, fliprConstraintsBuilder types.FliprConstraintsBuilder) types.MTLSHandler {
	return MTLSHandlerImpl{
		apiHandler:              apiHandler,
		fliprClient:             fliprClient,
		fliprConstraintsBuilder: fliprConstraintsBuilder,
	}
}

const (
	_projectNameKey = "project_name"
	_tierKey        = "tier"
)

const (
	_mTLSFliprName             = "enableMTLS"
	_mTLSRuntimeClassFliprName = "enableMTLS"
)

// EnableMTLS determines whether mTLS should be enabled based on project information
func (m MTLSHandlerImpl) EnableMTLS(projectName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), _mtlsTimeout)
	defer cancel()

	var project v2beta1pb.Project
	err := m.apiHandler.Get(ctx, projectName, projectName, &metav1.GetOptions{}, &project)
	if err != nil {
		return false, err
	}

	constraintsMap := make(map[string]interface{})
	constraintsMap[_projectNameKey] = projectName
	constraintsMap[_tierKey] = project.Spec.Tier
	fliprConstraints := m.fliprConstraintsBuilder.GetFliprConstraints(constraintsMap)

	enabled, err := m.fliprClient.GetBoolValue(ctx, _mTLSFliprName, fliprConstraints, false)
	if err != nil {
		return false, err
	}

	return enabled, nil
}

// EnableMTLSRuntimeClass determines whether the mTLS runtime class should be added to the jobs specs
func (m MTLSHandlerImpl) EnableMTLSRuntimeClass(projectName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), _mtlsTimeout)
	defer cancel()

	var project v2beta1pb.Project
	err := m.apiHandler.Get(ctx, projectName, projectName, &metav1.GetOptions{}, &project)
	if err != nil {
		return false, err
	}

	constraintsMap := make(map[string]interface{})
	constraintsMap[_projectNameKey] = projectName
	constraintsMap[_tierKey] = project.Spec.Tier
	fliprConstraints := m.fliprConstraintsBuilder.GetFliprConstraints(constraintsMap)

	enabled, err := m.fliprClient.GetBoolValue(ctx, _mTLSRuntimeClassFliprName, fliprConstraints, false)
	if err != nil {
		return false, err
	}

	return enabled, nil
}
