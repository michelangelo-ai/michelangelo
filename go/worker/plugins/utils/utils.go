package utils

import (
	"time"

	"github.com/cadence-workflow/starlark-worker/star"
	"github.com/cadence-workflow/starlark-worker/workflow"
	jsoniter "github.com/json-iterator/go"
	"go.starlark.net/starlark"
	"go.uber.org/yarpc/yarpcerrors"
)

// These are valid condition types of all Jobs
const (
	EnqueuedCondition      string = "Enqueued"
	KillingCondition       string = "Killing"
	KilledCondition        string = "Killed"
	LaunchedCondition      string = "Launched"
	PendingCondition       string = "Pending"
	ScheduledCondition     string = "Scheduled"
	SecretCreatedCondition string = "SecretCreated"
	SucceededCondition     string = "Succeeded"
)

// These are valid condition types of a Spark Job
const (
	SparkAppRunningCondition string = "SparkAppRunning"
	SparkAppFailedCondition  string = "SparkAppFailed"
)

const LongTimeout = time.Hour * 24 * 365 * 10 // 10 years, practically - no timeout
const LongRetry = time.Hour * 24 * 365 * 10   // 10 years, practically - no timeout

var DefaultNonRetriableErrorReasons = []string{
	"cadenceInternal:Panic",                  // panics
	"cadenceInternal:Generic",                // cadence converter errors (similar to invalid-argument)
	"400",                                    // bad-request https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/400
	"401",                                    // unauthorized
	"403",                                    // forbidden
	"404",                                    // not-found
	"405",                                    // method-not-allowed
	"502",                                    // bad-gateway
	yarpcerrors.CodeCancelled.String(),       // client error
	yarpcerrors.CodeNotFound.String(),        // client error
	yarpcerrors.CodeAlreadyExists.String(),   // client error
	yarpcerrors.CodeInvalidArgument.String(), // client error
	yarpcerrors.CodeUnauthenticated.String(), // client error
	yarpcerrors.CodePermissionDenied.String(), // client error
	yarpcerrors.CodeUnimplemented.String(),    // client error
	yarpcerrors.CodeDataLoss.String(),         // server error; unrecoverable data corruption
	yarpcerrors.CodeInternal.String(),         // server error; serious error, like panic
}

// DefaultRetryPolicy is the default retry policy for workflows with
// a 15-second initial interval and 5-minute expiration.
var DefaultRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 15,
	BackoffCoefficient:       1,
	ExpirationInterval:       time.Minute * 5,
	NonRetriableErrorReasons: DefaultNonRetriableErrorReasons,
	MaximumAttempts:          1,
}

// DefaultSensorRetryPolicy is the default retry policy for sensor workflows
// with a 10-second initial interval and long timeout for polling operations.
var DefaultSensorRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 10,
	BackoffCoefficient:       1,
	ExpirationInterval:       LongTimeout,
	NonRetriableErrorReasons: DefaultNonRetriableErrorReasons,
}

// AsStar converts a Go value to a Starlark value by marshaling through JSON.
func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return star.Decode(b, out)
}

// AsGo converts a Starlark value to a Go value by encoding through JSON.
func AsGo(source starlark.Value, out any) error {
	b, err := star.Encode(source)
	if err != nil {
		return err
	}
	return jsoniter.Unmarshal(b, out)
}
