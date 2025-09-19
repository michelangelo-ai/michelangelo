package deployment

// Events
const (
	_normalType            = "Normal"
	_stageChangeEvent      = "StageChange"
	_earlyTerminationEvent = "EarlyTermination"
)

// Log keys
const (
	_deploymentKey     = "deployment"
	_targetLoggingKey  = "target"
	_originalStageKey  = "original-stage"
	_newStageKey       = "new-stage"
	_providerStatus    = "provider-status"
	_desiredModelKey   = "desired-model"
	_candidateModelKey = "candidate-model"
	_currentModelKey   = "current-model"
)

// Metric tags
const (
	_namespaceTag = "namespace"
)
