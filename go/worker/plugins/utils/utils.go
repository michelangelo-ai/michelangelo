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

const CadenceLongTimeout = time.Hour * 24 * 365 * 10 // 10 years, practically - no timeout

var CadenceDefaultNonRetriableErrorReasons = []string{
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

var CadenceDefaultRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 15,
	BackoffCoefficient:       1,
	ExpirationInterval:       time.Minute * 5,
	NonRetriableErrorReasons: CadenceDefaultNonRetriableErrorReasons,
	MaximumAttempts:          1,
}

var CadenceDefaultSensorRetryPolicy = workflow.RetryPolicy{
	InitialInterval:          time.Second * 10,
	BackoffCoefficient:       1,
	ExpirationInterval:       CadenceLongTimeout,
	NonRetriableErrorReasons: CadenceDefaultNonRetriableErrorReasons,
	MaximumAttempts:          1,
}

func AsStar(source any, out any) error {
	b, err := jsoniter.Marshal(source)
	if err != nil {
		return err
	}
	return star.Decode(b, out)
}
func AsGo(source starlark.Value, out any) error {
	b, err := star.Encode(source)
	if err != nil {
		return err
	}
	return jsoniter.Unmarshal(b, out)
}
