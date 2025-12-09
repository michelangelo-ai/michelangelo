// Package apihook provides API hooks for project lifecycle management in Michelangelo.
// It handles project creation validation and ensures that Kubernetes namespaces are
// properly created and managed alongside Michelangelo projects.
package apihook

import (
	"context"
	"strings"

	"github.com/michelangelo-ai/michelangelo/go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sCoreClient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

const (
	defaultNamespace         = "default"
	integrationTestNamespace = "ma-integration-test"
	systemNamespacePrefix    = "kube-"
)

// RegisterProjectAPIHook registers the API hook for Project operations.
// It initializes a Kubernetes client from the provided REST config and registers
// the hook to intercept project API calls for validation and namespace management.
//
// The hook ensures that:
//   - Project names match their namespace names (except for integration tests)
//   - Projects are not created in default or system namespaces
//   - Kubernetes namespaces are created before projects
//
// Parameters:
//   - logger: zap logger for structured logging
//   - apiHandler: API handler for processing Michelangelo API requests
//   - k8sRestConfig: Kubernetes REST configuration for cluster access
//
// Returns an error if the Kubernetes client cannot be initialized.
func RegisterProjectAPIHook(logger *zap.Logger, apiHandler api.Handler, k8sRestConfig *rest.Config) error {
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

// BeforeCreate is called before a project is created and handles namespace creation
// and validation. It ensures that the project name matches the namespace name and
// creates the corresponding Kubernetes namespace if it doesn't already exist.
//
// Validation rules:
//   - Project name must match namespace name (except for ma-integration-test namespace)
//   - Projects cannot be created in the default namespace
//   - Projects cannot be created in system namespaces (prefixed with "kube-")
//
// If the namespace already exists, the method continues without error. This allows
// for idempotent project creation and handles cases where namespaces are pre-created.
//
// Returns an error if:
//   - The validation rules are violated (InvalidArgument or PermissionDenied)
//   - The namespace creation fails for reasons other than AlreadyExists
func (a apiHook) BeforeCreate(ctx context.Context, request *v2.CreateProjectRequest) error {
	// Validate the request
	if request.Project.Namespace != integrationTestNamespace && request.Project.Name != request.Project.Namespace {
		return status.Errorf(codes.InvalidArgument,
			"project name <%s> is different from namespace name <%s>. Project name must be the same as namespace name.",
			request.Project.Name,
			request.Project.Namespace)
	}

	if request.Project.Namespace == defaultNamespace || strings.HasPrefix(request.Project.Namespace, systemNamespacePrefix) {
		return status.Errorf(codes.PermissionDenied,
			"namespace <%s> is invalid. Users are forbidden to create projects in default or system namespace",
			request.Project.Namespace)
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
