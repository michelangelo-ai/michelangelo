package pipelinerunutils

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const testConditionType = "ExecuteWorkflow"

func runWithConditions(conditions ...*apipb.Condition) *v2.PipelineRun {
	return &v2.PipelineRun{Status: v2.PipelineRunStatus{Conditions: conditions}}
}

func TestTransientConditionStartsTheClockOnFirstFailure(t *testing.T) {
	run := runWithConditions()

	condition, err := TransientCondition(run, testConditionType,
		ReasonWorkflowStatusUnavailable, errors.New("service unavailable"), StartupRetryDeadline)

	require.NoError(t, err)
	require.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status)
	require.Equal(t, ReasonWorkflowStatusUnavailable, condition.Reason)
	require.Equal(t, "service unavailable", condition.Message)
	require.NotZero(t, condition.LastUpdatedTimestamp)
}

// The controller only writes status when it differs from what it read, and a
// status write feeds its own watch. Returning the stored condition untouched is
// what keeps a retry from re-triggering reconcile immediately.
func TestTransientConditionIsUnchangedWhileTheSameFailureRepeats(t *testing.T) {
	firstSeen := time.Now().Add(-time.Minute).Unix()
	existing := &apipb.Condition{
		Type:                 testConditionType,
		Status:               apipb.CONDITION_STATUS_UNKNOWN,
		Reason:               ReasonWorkflowStatusUnavailable,
		Message:              "service unavailable",
		LastUpdatedTimestamp: firstSeen,
	}
	run := runWithConditions(existing)

	condition, err := TransientCondition(run, testConditionType,
		ReasonWorkflowStatusUnavailable, errors.New("service unavailable (attempt 2)"), StartupRetryDeadline)

	require.NoError(t, err)
	require.Equal(t, firstSeen, condition.LastUpdatedTimestamp, "clock must not restart")
	require.Equal(t, "service unavailable", condition.Message, "message must stay stable")
	require.Equal(t, existing, condition)
}

func TestTransientConditionGivesUpOnceTheDeadlinePasses(t *testing.T) {
	run := runWithConditions(&apipb.Condition{
		Type:                 testConditionType,
		Status:               apipb.CONDITION_STATUS_UNKNOWN,
		Reason:               ReasonProjectFetchFailed,
		LastUpdatedTimestamp: time.Now().Add(-2 * StartupRetryDeadline).Unix(),
	})

	condition, err := TransientCondition(run, testConditionType,
		ReasonProjectFetchFailed, errors.New("api server down"), StartupRetryDeadline)

	require.Nil(t, condition)
	require.ErrorContains(t, err, "giving up")
	require.ErrorContains(t, err, "api server down")
}

func TestTransientConditionWithoutDeadlineRetriesForever(t *testing.T) {
	run := runWithConditions(&apipb.Condition{
		Type:                 testConditionType,
		Status:               apipb.CONDITION_STATUS_UNKNOWN,
		Reason:               ReasonWorkflowStatusUnavailable,
		LastUpdatedTimestamp: time.Now().Add(-30 * 24 * time.Hour).Unix(),
	})

	condition, err := TransientCondition(run, testConditionType,
		ReasonWorkflowStatusUnavailable, errors.New("still down"), NoRetryDeadline)

	require.NoError(t, err)
	require.Equal(t, apipb.CONDITION_STATUS_UNKNOWN, condition.Status)
}

// A different failure is a different clock: an old unrelated condition must not
// make a brand new problem look like it has already exhausted its deadline.
func TestTransientConditionRestartsTheClockForADifferentFailure(t *testing.T) {
	stale := time.Now().Add(-2 * StartupRetryDeadline).Unix()
	run := runWithConditions(&apipb.Condition{
		Type:                 testConditionType,
		Status:               apipb.CONDITION_STATUS_UNKNOWN,
		Reason:               ReasonProjectFetchFailed,
		LastUpdatedTimestamp: stale,
	})

	condition, err := TransientCondition(run, testConditionType,
		ReasonWorkflowStartFailed, errors.New("cadence unreachable"), StartupRetryDeadline)

	require.NoError(t, err)
	require.Equal(t, ReasonWorkflowStartFailed, condition.Reason)
	require.Greater(t, condition.LastUpdatedTimestamp, stale)
}

func TestIsTerminal(t *testing.T) {
	require.True(t, IsTerminal(Terminalf("malformed manifest")))
	require.True(t, IsTerminal(fmt.Errorf("wrapped: %w", Terminalf("malformed manifest"))))
	require.False(t, IsTerminal(errors.New("connection refused")))
	require.False(t, IsTerminal(nil))
}

func TestRetryableAPIError(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Resource: "projects"}, "missing")
	require.False(t, RetryableAPIError(notFound))

	require.True(t, RetryableAPIError(apierrors.NewTimeoutError("slow", 1)))
	require.True(t, RetryableAPIError(apierrors.NewTooManyRequestsError("throttled")))
	require.True(t, RetryableAPIError(errors.New("connection refused")))
	require.True(t, RetryableAPIError(apierrors.NewServiceUnavailable("down")))

	// Sanity: the helper is only meaningful for real API errors.
	require.True(t, RetryableAPIError(&apierrors.StatusError{ErrStatus: metav1.Status{Code: 500}}))
}
