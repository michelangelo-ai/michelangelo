package trigger

import (
	"github.com/cadence-workflow/starlark-worker/workflow"
	"github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	"go.uber.org/zap"
)

// BackfillTrigger workflow with provided trigger run spec
func (r *workflows) BackfillTrigger(ctx workflow.Context, req triggerrun.CreateTriggerRequest) (map[string]any, error) {
	ctx = workflow.WithBackend(ctx, r.workflow)
	ctx = workflow.WithActivityOptions(ctx, _activityOptionsDefault)
	tr := req.TriggerRun
	log := workflow.GetLogger(ctx).With(
		zap.String("trigger_run", tr.Name),
		zap.String("namespace", tr.Namespace),
	)
	logicalTs := workflow.Now(ctx).UTC()
	ctx = workflow.WithValue(ctx, contextKeylogicalTs, logicalTs)
	triggerContext := Object{
		"DS":            logicalTs.Format("2006-01-02"),
		"StartedAt":     workflow.Now(ctx),
		"TriggeredRuns": []parameter.BackfillParam{},
	}
	ctx = workflow.WithValue(ctx, contextKeyTriggerContext, triggerContext)
	// setup query handler for runHistory
	if err := workflow.SetQueryHandler(ctx, "triggerContext", func() (map[string]any, error) {
		return triggerContext, nil
	}); err != nil {
		log.Error("failed to set query handler for triggerContext", zap.Error(err))
		return nil, err
	}
	log.Info("backfill trigger workflow started", zap.String("operation", "backfill_trigger_workflow"))
	var err error
	if tr.Spec.Trigger.MaxConcurrency > 0 {
		err = concurrentRun(ctx, tr)
	} else {
		err = batchRun(ctx, tr)
	}
	if err != nil {
		return nil, err
	}
	triggerContext["FinishedAt"] = workflow.Now(ctx)
	return triggerContext, nil
}
