package deployment

// Events
const (
	normalType            = "Normal"
	stageChangeEvent      = "StageChange"
	earlyTerminationEvent = "EarlyTermination"
)

// Log keys
const (
	deploymentKey     = "deployment"
	targetLoggingKey  = "target"
	originalStageKey  = "original-stage"
	newStageKey       = "new-stage"
	providerStatus    = "provider-status"
	desiredModelKey   = "desired-model"
	candidateModelKey = "candidate-model"
	currentModelKey   = "current-model"
)

// Metric tags
const (
	_namespaceTag = "namespace"
)
