package apihook

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api/utils"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

// BeforeDelete refuses to delete a TriggerRun that is still live. Deleting it
// would remove the record of what was running while its schedule and child
// runs are still being torn down; killing it first keeps a KILLED object and
// cancels the in-flight runs. A TriggerRun that does not exist is left to the
// delete itself, which reports NotFound.
func (a apiHook) BeforeDelete(ctx context.Context, request *v2.DeleteTriggerRunRequest) error {
	run := &v2.TriggerRun{}
	if err := a.apiHandler.Get(ctx, request.GetNamespace(), request.GetName(), &metav1.GetOptions{}, run); err != nil {
		if utils.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	return rejectIfLive(run)
}

// BeforeDeleteCollection applies the same rule to every TriggerRun the request
// selects. The request's list options are honoured, so a selector-scoped
// delete only checks the objects it would remove; the whole request is refused
// if any of them is live.
func (a apiHook) BeforeDeleteCollection(ctx context.Context, request *v2.DeleteTriggerRunCollectionRequest) error {
	listOptions := request.GetListOptions()
	if listOptions == nil {
		listOptions = &metav1.ListOptions{}
	}
	runs := &v2.TriggerRunList{}
	if err := a.apiHandler.List(ctx, request.GetNamespace(), listOptions, nil, runs); err != nil {
		return err
	}
	for i := range runs.Items {
		if err := rejectIfLive(&runs.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

func rejectIfLive(run *v2.TriggerRun) error {
	if triggerrun.IsTerminalState(run.Status.State) {
		return nil
	}
	return status.Errorf(codes.FailedPrecondition,
		"TriggerRun %s/%s is %s and cannot be deleted: only a FAILED, KILLED or SUCCEEDED TriggerRun can be deleted; "+
			"set spec.action to TRIGGER_RUN_ACTION_KILL to stop it first",
		run.Namespace, run.Name, run.Status.State)
}
