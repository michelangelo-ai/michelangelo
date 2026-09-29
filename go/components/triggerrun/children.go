package triggerrun

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	apiutils "github.com/michelangelo-ai/michelangelo/go/api/utils"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// TriggeredByLabel is stamped on every PipelineRun a trigger dispatches, valued with
// the TriggerRun name. Child runs carry no ownerReference to their trigger, so this
// label is the only link used to find them when the trigger is killed or deleted.
const TriggeredByLabel = "pipelinerun.michelangelo/triggered-by"

// isPipelineRunTerminal reports whether a PipelineRun has finished and needs no cancel.
func isPipelineRunTerminal(run *v2pb.PipelineRun) bool {
	switch run.Status.State {
	case v2pb.PIPELINE_RUN_STATE_SUCCEEDED,
		v2pb.PIPELINE_RUN_STATE_KILLED,
		v2pb.PIPELINE_RUN_STATE_FAILED,
		v2pb.PIPELINE_RUN_STATE_SKIPPED:
		return true
	}
	return false
}

// cancelActiveChildPipelineRuns requests a kill (Spec.Kill) on every non-terminal
// PipelineRun dispatched by the trigger and returns how many are still active.
//
// It is idempotent so the drain can call it on every loop: children already flagged
// for kill are counted but not updated again. Each candidate is re-read before the
// update so a stale list (e.g. from metadata storage) neither conflicts nor re-kills
// a run that has since finished. Per-child failures are joined and returned after
// the remaining children have been attempted.
func (r *Reconciler) cancelActiveChildPipelineRuns(
	ctx context.Context, log logr.Logger, triggerRun *v2pb.TriggerRun,
) (int, error) {
	list := &v2pb.PipelineRunList{}
	opts := &metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{TriggeredByLabel: triggerRun.Name}).String(),
	}
	if err := r.List(ctx, triggerRun.Namespace, opts, nil, list); err != nil {
		return 0, fmt.Errorf("list child pipeline runs of trigger run %s/%s: %w",
			triggerRun.Namespace, triggerRun.Name, err)
	}

	active := 0
	var errs []error
	for i := range list.Items {
		if isPipelineRunTerminal(&list.Items[i]) {
			continue
		}
		child := &v2pb.PipelineRun{}
		if err := r.Get(ctx, triggerRun.Namespace, list.Items[i].Name, &metav1.GetOptions{}, child); err != nil {
			if apiutils.IsNotFoundError(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("get child pipeline run %s: %w", list.Items[i].Name, err))
			continue
		}
		if isPipelineRunTerminal(child) {
			continue
		}
		active++
		if child.Spec.Kill {
			continue
		}
		child.Spec.Kill = true
		if err := r.Update(ctx, child, &metav1.UpdateOptions{}); err != nil {
			errs = append(errs, fmt.Errorf("request kill for child pipeline run %s: %w", child.Name, err))
			continue
		}
		log.Info("requested kill for child pipeline run", "pipelineRun", child.Name, "state", child.Status.State.String())
	}
	return active, errors.Join(errs...)
}
