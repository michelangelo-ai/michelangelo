package apihook

import (
	"context"
	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sCoreClient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"strings"
)

const (
	_defaultNamespace         = "default"
	_integrationTestNamespace = "ma-integration-test"
	_systemNamespacePrefix    = "kube-"
)

type FxProjectAPIHookResult struct {
	fx.Out
	APIHook v2.ProjectAPIHook `group:"apiHooks"`
}

// GetProjectAPIHook returns the API hook for Project
func GetProjectAPIHook(logger *zap.Logger, apiHandler api.Handler, k8sRestConfig *rest.Config) (FxProjectAPIHookResult, error) {
	k8sClient, err := k8sCoreClient.NewForConfig(k8sRestConfig)
	if err != nil {
		return FxProjectAPIHookResult{}, err
	}
	return FxProjectAPIHookResult{
		APIHook: apiHook{
			logger:     logger,
			apiHandler: apiHandler,
			k8sClient:  k8sClient,
		},
	}, nil
}

type apiHook struct {
	v2.NoopProjectAPIHook
	logger     *zap.Logger
	apiHandler api.Handler
	k8sClient  k8sCoreClient.CoreV1Interface
}

// BeforeCreate creates a new namespace of the same name before creating a project
func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateProjectRequest) error {
	// Validate the request
	if request.Project.Namespace != _integrationTestNamespace && request.Project.Name != request.Project.Namespace {
		return status.Errorf(codes.InvalidArgument, "project name and namespace cannot be different")
	}

	if request.Project.Namespace == _defaultNamespace || strings.HasPrefix(request.Project.Namespace, _systemNamespacePrefix) {
		return status.Errorf(codes.PermissionDenied, "users are forbidden to create project in default or system namespace")
	}

	// Create namespace
	namespace := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "namespace",
			APIVersion: "core/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: request.Project.Namespace,
		},
		Spec: corev1.NamespaceSpec{},
	}
	resp, err := a.k8sClient.Namespaces().Create(ctx, &namespace, metav1.CreateOptions{})

	// if namespace already exist, we continue with project creation
	if k8sErrors.IsAlreadyExists(err) {
		a.logger.Info("Namespace already exists.")
		return nil
	}
	if err != nil {
		a.logger.Error("Fail to create namespace", zap.Error(err))
		return api.K8sError2GrpcError(err, "failed to create namespace")
	}

	a.logger.Info("Successfully create namespace", zap.String("namespace", request.Project.Namespace), zap.Any("response", resp))
	return nil
}
