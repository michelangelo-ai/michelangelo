package inferenceserver

import (
	"context"
	"fmt"
	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/michelangelo-ai/michelangelo/go/base/env"
	"github.com/michelangelo-ai/michelangelo/go/inferenceserver/provider/serving"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const requeueAfter = 10 * time.Second

type Reconciler struct {
	client.Client
	tritonProvider serving.Provider
	llmdProvider   serving.Provider
	env            env.Context
}

func (r *Reconciler) Register(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2pb.InferenceServer{}).
		Complete(r)
}

// getProvider returns the appropriate serving provider based on the backend type
func (r *Reconciler) getProvider(backendType v2pb.BackendType) (serving.Provider, error) {
	switch backendType {
	case v2pb.BACKEND_TYPE_TRITON:
		if r.tritonProvider == nil {
			return nil, fmt.Errorf("triton provider not available")
		}
		return r.tritonProvider, nil
	case v2pb.BACKEND_TYPE_LLM_D:
		if r.llmdProvider == nil {
			return nil, fmt.Errorf("llm-d provider not available")
		}
		return r.llmdProvider, nil
	default:
		return nil, fmt.Errorf("unsupported backend type: %v", backendType)
	}
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	res := ctrl.Result{}
	// Fetch the InferenceServer CRD
	inferenceServer := &v2pb.InferenceServer{}
	err := r.Client.Get(ctx, req.NamespacedName, inferenceServer)
	if err != nil {
		if utils.IsNotFoundError(err) {
			return res, nil
		}
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("failed to fetch InferenceServer: %w", err)
	}
	original := inferenceServer.DeepCopy()
	configMapName := fmt.Sprintf("%s-model-config", inferenceServer.Name)

	// Get the appropriate provider based on backend type
	backendType := inferenceServer.Spec.BackendType
	provider, err := r.getProvider(backendType)
	if err != nil {
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("failed to get provider for backend type %v: %w", backendType, err)
	}

	// Check for model reload trigger annotation
	if triggerValue, exists := inferenceServer.GetMetadata().GetAnnotations()["michelangelo.ai/model-reload-trigger"]; exists {
		logger.Info("Model reload trigger detected", "triggerValue", triggerValue)

		// Trigger model reload/update in the provider
		err = provider.UpdateInferenceServer(ctx, logger, inferenceServer.GetMetadata().GetName(), inferenceServer.GetMetadata().GetNamespace())
		if err != nil {
			logger.Error(err, "Failed to trigger model reload")
			res.RequeueAfter = requeueAfter
		} else {
			logger.Info("Model reload triggered successfully")
			// Note: We don't remove the trigger annotation here to allow for idempotent retries
			// The deployment controller can clean up old annotations if needed
		}
	}

	// Check if infrastructure already exists and update status
	err = provider.GetStatus(ctx, logger, inferenceServer)
	if err != nil {
		res.RequeueAfter = requeueAfter
		return res, fmt.Errorf("failed to check existing infrastructure: %w", err)
	}

	// If infrastructure doesn't exist (status is INITIALIZED), create it
	if inferenceServer.Status.State == v2pb.INFERENCE_SERVER_STATE_INITIALIZED {
		logger.Info("Creating new infrastructure", "backendType", backendType)

		// Create ConfigMap first (only for certain backend types)
		if backendType == v2pb.BACKEND_TYPE_TRITON || backendType == v2pb.BACKEND_TYPE_LLM_D {
			logger.Info("Creating model config ConfigMap")
			err = r.createModelConfigMap(ctx, configMapName, logger, inferenceServer, backendType)
			if err != nil {
				res.RequeueAfter = requeueAfter
				return res, fmt.Errorf("failed to create model config ConfigMap: %w", err)
			}
		}

		// Create serving infrastructure
		logger.Info("Creating serving infrastructure")
		err = provider.CreateInferenceServer(ctx, logger, inferenceServer.Name, inferenceServer.Namespace, configMapName)
		if err != nil {
			res.RequeueAfter = requeueAfter
			return res, fmt.Errorf("failed to create serving infrastructure: %w", err)
		}

		// Update status to creating
		inferenceServer.Status.State = v2pb.INFERENCE_SERVER_STATE_CREATING
		inferenceServer.Status.UpdateTime = fmt.Sprintf("%d", time.Now().Unix())
	} else {
		logger.Info("Infrastructure already exists", "backendType", backendType, "state", inferenceServer.Status.State)
	}

	if !reflect.DeepEqual(original, inferenceServer) {
		if err := r.Status().Update(ctx, inferenceServer); err != nil {
			logger.Error(err, "failed to update Deployment status")
			res.RequeueAfter = requeueAfter
			return res, err
		}
	}

	return res, nil
}

func (r *Reconciler) createModelConfigMap(ctx context.Context, configMapName string, log logr.Logger, inferenceServer *v2pb.InferenceServer, backendType v2pb.BackendType) error {
	name := inferenceServer.GetMetadata().GetName()
	namespace := inferenceServer.GetMetadata().GetNamespace()

	// Create empty model list initially - will be populated by deployment controller
	modelListJSON := "[]"

	// Determine provider label based on backend type
	providerLabel := "triton" // default
	switch backendType {
	case v2pb.BACKEND_TYPE_TRITON:
		providerLabel = "triton"
	case v2pb.BACKEND_TYPE_LLM_D:
		providerLabel = "llm-d"
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"michelangelo.ai/inference": name,
				"michelangelo.ai/provider":  providerLabel,
			},
		},
		Data: map[string]string{
			"model-list.json": modelListJSON,
		},
	}

	log.Info("Creating ConfigMap", "name", configMapName, "namespace", namespace)
	err := r.Client.Create(ctx, configMap)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			log.Info("ConfigMap already exists", "name", configMapName)
			return nil
		}
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	log.Info("ConfigMap created successfully", "name", configMapName)
	return nil
}
