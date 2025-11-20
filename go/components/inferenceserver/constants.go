package inferenceserver

import (
	"time"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

// Controller configuration constants
const (
	// ControllerName is the name of the inference server controller
	ControllerName = "inference-server-controller"

	// Team responsible for this controller
	Team = "ml-serving"

	// Finalizer to ensure proper cleanup
	FinalizerName = "inferenceservers.michelangelo.api/finalizer"

	// Timeout for reconciliation operations
	ReconcilerTimeout = 1 * time.Minute

	// Timeout for condition engine operations
	EngineTimeout = 50 * time.Second

	// Requeue period for active reconciliation
	ActiveRequeueAfter = 1 * time.Minute

	// Requeue period for steady state
	SteadyStateRequeueAfter = 10 * time.Minute
)

// Condition types for inference server lifecycle
const (
	// Creation phase conditions
	ConditionInitialization     = "Initialization"
	ConditionServiceCreation    = "ServiceCreation"
	ConditionDeploymentCreation = "DeploymentCreation"
	ConditionNetworkSetup       = "NetworkSetup"
	ConditionHealthCheck        = "HealthCheck"

	// Deletion phase conditions
	ConditionScaleDown          = "ScaleDown"
	ConditionServiceDeletion    = "ServiceDeletion"
	ConditionDeploymentDeletion = "DeploymentDeletion"
	ConditionCleanup            = "Cleanup"
)

// Inference server states - using the correct protobuf enum values
const (
	StateCreating = v2pb.INFERENCE_SERVER_STATE_CREATING
	StateServing  = v2pb.INFERENCE_SERVER_STATE_SERVING
	StateDeleting = v2pb.INFERENCE_SERVER_STATE_DELETING
	StateDeleted  = v2pb.INFERENCE_SERVER_STATE_DELETED
	StateFailed   = v2pb.INFERENCE_SERVER_STATE_FAILED
)

// Backend types - using the correct protobuf enum values
const (
	BackendTriton = v2pb.BACKEND_TYPE_TRITON
	BackendLLMD   = v2pb.BACKEND_TYPE_LLM_D
	BackendDynamo = v2pb.BACKEND_TYPE_DYNAMO
)
