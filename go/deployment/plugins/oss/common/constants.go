package common

// Actor types for OSS deployment
const (
	ActorTypeValidation      = "Validation"
	ActorTypeResourcePrep    = "ResourcePreparation"
	ActorTypeModelLoad       = "ModelLoad"
	ActorTypeHealthCheck     = "HealthCheck"
	ActorTypeCleanup         = "Cleanup"
	ActorTypeRollback        = "Rollback"
	ActorTypeSteadyState     = "SteadyState"
)

// Event types
const (
	EventReasonValidationStarted    = "ValidationStarted"
	EventReasonValidationCompleted  = "ValidationCompleted"
	EventReasonValidationFailed     = "ValidationFailed"
	EventReasonResourcePrepStarted  = "ResourcePrepStarted"
	EventReasonResourcePrepCompleted = "ResourcePrepCompleted"
	EventReasonResourcePrepFailed   = "ResourcePrepFailed"
	EventReasonModelLoadStarted     = "ModelLoadStarted"
	EventReasonModelLoadCompleted   = "ModelLoadCompleted"
	EventReasonModelLoadFailed      = "ModelLoadFailed"
	EventReasonCleanupStarted       = "CleanupStarted"
	EventReasonCleanupCompleted     = "CleanupCompleted"
	EventReasonCleanupFailed        = "CleanupFailed"
	EventReasonRollbackStarted      = "RollbackStarted"
	EventReasonRollbackCompleted    = "RollbackCompleted"
	EventReasonRollbackFailed       = "RollbackFailed"
)

// Messages
const (
	MessageValidationStarted      = "Starting deployment validation"
	MessageValidationCompleted    = "Deployment validation completed successfully"
	MessageValidationFailed       = "Deployment validation failed"
	MessageResourcePrepStarted    = "Starting resource preparation"
	MessageResourcePrepCompleted  = "Resource preparation completed successfully"
	MessageResourcePrepFailed     = "Resource preparation failed"
	MessageModelLoadStarted       = "Starting model loading"
	MessageModelLoadCompleted     = "Model loading completed successfully"
	MessageModelLoadFailed        = "Model loading failed"
	MessageCleanupStarted         = "Starting deployment cleanup"
	MessageCleanupCompleted       = "Deployment cleanup completed successfully"
	MessageCleanupFailed          = "Deployment cleanup failed"
	MessageRollbackStarted        = "Starting deployment rollback"
	MessageRollbackCompleted      = "Deployment rollback completed successfully"
	MessageRollbackFailed         = "Deployment rollback failed"
)