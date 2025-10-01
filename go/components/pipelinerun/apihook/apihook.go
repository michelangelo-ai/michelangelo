package apihook

import (
	"context"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	k8sCoreClient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

// RegisterPipelineRunAPIHook returns the API hook for Project
func RegisterPipelineRunAPIHook(logger *zap.Logger, apiHandler api.Handler, k8sRestConfig *rest.Config) error {
	k8sClient, err := k8sCoreClient.NewForConfig(k8sRestConfig)
	if err != nil {
		return err
	}
	v2.RegisterProjectAPIHook(apiHook{
		logger:     logger,
		apiHandler: apiHandler,
		k8sClient:  k8sClient,
	})
	return nil
}

type apiHook struct {
	v2.NoopProjectAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	k8sClient  k8sCoreClient.CoreV1Interface
}

// BeforeCreate creates a new namespace of the same name before creating a project
func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateProjectRequest) error {
	return nil
}
