package inferenceserver

// Event reasons for inference server lifecycle
const (
	// Creation events
	EventReasonCreationStarted   = "CreationStarted"
	EventReasonServiceCreated    = "ServiceCreated"
	EventReasonDeploymentCreated = "DeploymentCreated"
	EventReasonHealthCheckPassed = "HealthCheckPassed"
	EventReasonCreationCompleted = "CreationCompleted"

	// Deletion events
	EventReasonDeletionStarted   = "DeletionStarted"
	EventReasonScaledDown        = "ScaledDown"
	EventReasonServiceDeleted    = "ServiceDeleted"
	EventReasonDeploymentDeleted = "DeploymentDeleted"
	EventReasonDeletionCompleted = "DeletionCompleted"

	// Error events
	EventReasonCreationFailed    = "CreationFailed"
	EventReasonDeletionFailed    = "DeletionFailed"
	EventReasonHealthCheckFailed = "HealthCheckFailed"
	EventReasonTimeout           = "Timeout"
)

// Event messages
const (
	// Creation messages
	MessageCreationStarted   = "Started creating inference server"
	MessageServiceCreated    = "Service created successfully"
	MessageDeploymentCreated = "Deployment created successfully"
	MessageHealthCheckPassed = "Health check passed"
	MessageCreationCompleted = "Inference server creation completed"

	// Deletion messages
	MessageDeletionStarted   = "Started deleting inference server"
	MessageScaledDown        = "Scaled down to zero replicas"
	MessageServiceDeleted    = "Service deleted successfully"
	MessageDeploymentDeleted = "Deployment deleted successfully"
	MessageDeletionCompleted = "Inference server deletion completed"

	// Error messages
	MessageCreationFailed    = "Failed to create inference server"
	MessageDeletionFailed    = "Failed to delete inference server"
	MessageHealthCheckFailed = "Health check failed"
	MessageTimeout           = "Operation timed out"
)
