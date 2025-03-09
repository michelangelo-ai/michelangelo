package ext

import (
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

const TemporalLongTimeout = time.Hour * 24 * 365 * 10 // 10 years, practically - no timeout

var TemporalDefaultNonRetriableErrorReasons = []string{
	"temporalInternal:Panic",   // panics
	"temporalInternal:Generic", // Temporal converter errors (similar to invalid-argument)
	"400",                      // bad-request
	"401",                      // unauthorized
	"403",                      // forbidden
	"404",                      // not-found
	"405",                      // method-not-allowed
	"502",                      // bad-gateway
	"Cancelled",                // client error
	"NotFound",                 // client error
	"AlreadyExists",            // client error
	"InvalidArgument",          // client error
	"Unauthenticated",          // client error
	"PermissionDenied",         // client error
	"Unimplemented",            // client error
	"DataLoss",                 // server error; unrecoverable data corruption
	"Internal",                 // server error; serious error, like panic
}

var TemporalDefaultRetryPolicy = temporal.RetryPolicy{
	InitialInterval:        time.Second * 15,
	BackoffCoefficient:     1,
	MaximumAttempts:        5,
	NonRetryableErrorTypes: []string{"CustomNonRetryableError"},
}

var TemporalDefaultSensorRetryPolicy = temporal.RetryPolicy{
	InitialInterval:        time.Second * 10,
	BackoffCoefficient:     1,
	MaximumAttempts:        500000,
	NonRetryableErrorTypes: []string{"CustomNonRetryableError"},
}

var TemporalDefaultActivityOptions = workflow.ActivityOptions{
	ScheduleToStartTimeout: time.Second * 15,
	StartToCloseTimeout:    time.Second * 300,
	RetryPolicy:            &TemporalDefaultRetryPolicy,
}

var TemporalDefaultChildWorkflowOptions = workflow.ChildWorkflowOptions{
	WorkflowExecutionTimeout: TemporalLongTimeout,
	// Don't retry child workflows by default. Assumptions:
	// 1. Temporal scheduler starts child workflow reliably.
	// 2. Child workflow performs retries for all the activities it invokes.
	RetryPolicy: nil,
}
