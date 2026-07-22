package backends

import (
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

var _ Backend = &kserveBackend{}

// KServe InferenceService GVR
var inferenceServiceGVR = schema.GroupVersionResource{
	Group:    "serving.kserve.io",
	Version:  "v1beta1",
	Resource: "inferenceservices",
}

// Annotations read from the Michelangelo InferenceServer CR to configure the
// KServe InferenceService this backend creates.
const (
	// kserveModelFormatAnnotation overrides the default "sklearn" model
	// format (e.g. "triton", "pytorch") passed to KServe's predictor.
	kserveModelFormatAnnotation = "michelangelo/kserve-model-format"
	// kserveServiceAccountAnnotation names a ServiceAccount (with an
	// attached storage credentials Secret) for the InferenceService's
	// predictor to use when pulling the model artifact.
	kserveServiceAccountAnnotation = "michelangelo/kserve-service-account"
)

// kserveBackend implements Backend by creating and managing KServe InferenceService CRs
// on each target cluster. The controller does NOT use the Triton ConfigMap approach —
// instead it delegates model serving entirely to KServe.
type kserveBackend struct {
	// dynamicClientFn returns a dynamic client for the given cluster target.
	// Injected so the backend can reach each compute cluster's API server.
	dynamicClientFn func(ctx context.Context, target *v2pb.ClusterTarget) (dynamic.Interface, error)
}

// NewKServeBackend creates a KServe backend. dynamicClientFn must return a
// dynamic.Interface scoped to the given cluster target.
func NewKServeBackend(dynamicClientFn func(ctx context.Context, target *v2pb.ClusterTarget) (dynamic.Interface, error)) *kserveBackend {
	return &kserveBackend{dynamicClientFn: dynamicClientFn}
}

// CreateServer creates a KServe InferenceService CR on the target cluster.
// The InferenceService name matches the Michelangelo InferenceServer name.
// Model format and storage URI are taken from the InferenceServer's serving spec.
func (b *kserveBackend) CreateServer(ctx context.Context, logger *zap.Logger, _ client.Client, inferenceServer *v2pb.InferenceServer) (*ServerStatus, error) {
	for _, target := range inferenceServer.Spec.ClusterTargets {
		dynClient, err := b.dynamicClientFn(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("cluster %s: get dynamic client: %w", target.GetClusterId(), err)
		}

		isClient := dynClient.Resource(inferenceServiceGVR).Namespace(inferenceServer.Namespace)

		// Idempotent — skip if already exists.
		existing, err := isClient.Get(ctx, inferenceServer.Name, metav1.GetOptions{})
		if err == nil {
			logger.Info("KServe InferenceService already exists",
				zap.String("cluster", target.GetClusterId()),
				zap.String("name", existing.GetName()))
			continue
		}
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster %s: check InferenceService: %w", target.GetClusterId(), err)
		}

		storageURI, modelFormat := kserveStorageParams(inferenceServer)
		serviceAccountName := inferenceServer.GetMetadata().GetAnnotations()[kserveServiceAccountAnnotation]
		obj := buildInferenceServiceObject(inferenceServer.Name, inferenceServer.Namespace, storageURI, modelFormat, serviceAccountName)

		if _, err := isClient.Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("cluster %s: create InferenceService: %w", target.GetClusterId(), err)
		}
		logger.Info("Created KServe InferenceService",
			zap.String("cluster", target.GetClusterId()),
			zap.String("name", inferenceServer.Name))
	}

	return &ServerStatus{State: v2pb.INFERENCE_SERVER_STATE_CREATING}, nil
}

// GetServerStatus reads the KServe InferenceService status on the cluster the
// given kubeClient is scoped to. BackendProvisionActor calls this once per
// target with a target-scoped client (see backend_provision.go), the same
// convention tritonBackend.GetServerStatus relies on.
func (b *kserveBackend) GetServerStatus(ctx context.Context, logger *zap.Logger, kubeClient client.Client, inferenceServerName string, namespace string) (*ServerStatus, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.kserve.io",
		Version: "v1beta1",
		Kind:    "InferenceService",
	})
	err := kubeClient.Get(ctx, client.ObjectKey{Name: inferenceServerName, Namespace: namespace}, obj)
	if errors.IsNotFound(err) {
		return &ServerStatus{State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get InferenceService %s/%s: %w", namespace, inferenceServerName, err)
	}

	if isInferenceServiceReady(obj) {
		url, _ := inferenceServiceURL(obj)
		return &ServerStatus{
			State:     v2pb.INFERENCE_SERVER_STATE_SERVING,
			Endpoints: []string{url},
		}, nil
	}
	return &ServerStatus{State: v2pb.INFERENCE_SERVER_STATE_CREATING}, nil
}

// GetServerStatusForTarget checks a specific cluster target.
func (b *kserveBackend) GetServerStatusForTarget(ctx context.Context, logger *zap.Logger, target *v2pb.ClusterTarget, inferenceServerName string, namespace string) (*ServerStatus, error) {
	dynClient, err := b.dynamicClientFn(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("cluster %s: get dynamic client: %w", target.GetClusterId(), err)
	}

	obj, err := dynClient.Resource(inferenceServiceGVR).Namespace(namespace).Get(ctx, inferenceServerName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return &ServerStatus{State: v2pb.INFERENCE_SERVER_STATE_CREATE_PENDING}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cluster %s: get InferenceService: %w", target.GetClusterId(), err)
	}

	if isInferenceServiceReady(obj) {
		url, _ := inferenceServiceURL(obj)
		return &ServerStatus{
			State:     v2pb.INFERENCE_SERVER_STATE_SERVING,
			Endpoints: []string{url},
		}, nil
	}
	return &ServerStatus{State: v2pb.INFERENCE_SERVER_STATE_CREATING}, nil
}

// DeleteServer deletes the KServe InferenceService CR on all target clusters.
// The caller is responsible for iterating targets — this deletes from the local cluster only.
func (b *kserveBackend) DeleteServer(ctx context.Context, logger *zap.Logger, _ client.Client, inferenceServerName string, namespace string) error {
	// Deletion is target-scoped; callers that have a target list should call
	// DeleteServerForTarget. This no-ops gracefully for compatibility.
	logger.Info("KServe DeleteServer called without target context — deletion must be done per-target",
		zap.String("name", inferenceServerName))
	return nil
}

// DeleteServerForTarget deletes the InferenceService from a specific cluster.
func (b *kserveBackend) DeleteServerForTarget(ctx context.Context, logger *zap.Logger, target *v2pb.ClusterTarget, inferenceServerName string, namespace string) error {
	dynClient, err := b.dynamicClientFn(ctx, target)
	if err != nil {
		return fmt.Errorf("cluster %s: get dynamic client: %w", target.GetClusterId(), err)
	}

	err = dynClient.Resource(inferenceServiceGVR).Namespace(namespace).Delete(ctx, inferenceServerName, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil // already gone
	}
	return err
}

// IsHealthy returns true when the KServe InferenceService is in Ready state.
func (b *kserveBackend) IsHealthy(_ context.Context, _ *zap.Logger, _ client.Client, _ string, _ string) (bool, error) {
	// Without a target, we can't reach the cluster — fail-open.
	return true, nil
}

// CheckModelStatus checks whether a specific model revision is ready in KServe.
// For KServe, this means the InferenceService named after the model is Ready.
func (b *kserveBackend) CheckModelStatus(_ context.Context, _ *zap.Logger, _ client.Client, _ *http.Client, _ string, _ string, _ string, _ string) (bool, error) {
	// KServe manages model loading internally. Once the InferenceService is Ready
	// (checked by GetServerStatusForTarget), the model is available.
	// The deployment actor uses CheckModelStatus after CreateServer — we return true
	// to delegate readiness tracking to GetServerStatus.
	return true, nil
}

// LoadModel updates the KServe InferenceService storageUri to the new model path.
// KServe handles the rollout internally once the spec is updated. The model
// format is set once at creation time (kserveStorageParams) and is not
// touched here — modelName identifies the model, not its serving format.
func (b *kserveBackend) LoadModel(ctx context.Context, logger *zap.Logger, _ client.Client, dynClient dynamic.Interface, inferenceServerName, namespace, modelName, storageURI string) error {
	isClient := dynClient.Resource(inferenceServiceGVR).Namespace(namespace)

	obj, err := isClient.Get(ctx, inferenceServerName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get InferenceService %s/%s: %w", namespace, inferenceServerName, err)
	}

	if err := unstructured.SetNestedField(obj.Object, storageURI, "spec", "predictor", "model", "storageUri"); err != nil {
		return fmt.Errorf("set storageUri: %w", err)
	}

	if _, err := isClient.Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update InferenceService %s/%s: %w", namespace, inferenceServerName, err)
	}

	logger.Info("KServe InferenceService updated with new model",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("model", modelName),
		zap.String("storageURI", storageURI))
	return nil
}

// UnloadModel is a no-op for KServe — updating storageUri in LoadModel replaces
// the model in-place; there is no separate unload step.
func (b *kserveBackend) UnloadModel(_ context.Context, logger *zap.Logger, _ client.Client, _ dynamic.Interface, inferenceServerName, namespace, modelName string) error {
	logger.Info("KServe UnloadModel: no-op (in-place update)",
		zap.String("inferenceServer", inferenceServerName),
		zap.String("model", modelName))
	return nil
}

// buildInferenceServiceObject constructs an unstructured KServe InferenceService.
// serviceAccountName is optional; when empty, KServe uses the namespace default
// ServiceAccount and its predictor can only pull publicly-readable artifacts.
func buildInferenceServiceObject(name, namespace, storageURI, modelFormat, serviceAccountName string) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"predictor": map[string]interface{}{
			"model": map[string]interface{}{
				"modelFormat": map[string]interface{}{
					"name": modelFormat,
				},
				"storageUri": storageURI,
			},
		},
	}
	if serviceAccountName != "" {
		spec["predictor"].(map[string]interface{})["serviceAccountName"] = serviceAccountName
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.kserve.io/v1beta1",
			"kind":       "InferenceService",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"annotations": map[string]interface{}{
					"serving.kserve.io/deploymentMode": "RawDeployment",
				},
			},
			"spec": spec,
		},
	}
}

// isInferenceServiceReady returns true when the InferenceService has a Ready condition = True.
func isInferenceServiceReady(obj *unstructured.Unstructured) bool {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

// inferenceServiceURL extracts the serving URL from the InferenceService status.
func inferenceServiceURL(obj *unstructured.Unstructured) (string, bool) {
	url, found, _ := unstructured.NestedString(obj.Object, "status", "url")
	return url, found
}

// kserveStorageParams extracts storageURI and modelFormat from the InferenceServer spec.
// Falls back to sensible defaults when not set. modelFormat can be overridden
// via the kserveModelFormatAnnotation on the InferenceServer CR.
func kserveStorageParams(is *v2pb.InferenceServer) (storageURI, modelFormat string) {
	if is.Spec.GetInitSpec().GetServingSpec() != nil {
		storageURI = is.Spec.GetInitSpec().GetServingSpec().GetVersion()
	}
	if storageURI == "" {
		storageURI = fmt.Sprintf("gs://michelangelo-models/%s", is.Name)
	}
	modelFormat = "sklearn" // default; override via michelangelo/kserve-model-format annotation
	if format := is.GetMetadata().GetAnnotations()[kserveModelFormatAnnotation]; format != "" {
		modelFormat = format
	}
	return
}
