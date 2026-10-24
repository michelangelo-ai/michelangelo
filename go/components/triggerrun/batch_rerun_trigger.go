package triggerrun

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	clientInterface "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	uberconfig "go.uber.org/config"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// batchRerunTrigger implements the Runner interface for one-time batch rerun workflows.
//
// A batch rerun creates a new PipelineRun, resumed from a prior run, for each entry in
// TriggerRun.Spec.Trigger.BatchRerun.PipelineRuns. Like backfill, it runs once and completes;
// it does not have a recurring schedule.
type batchRerunTrigger struct {
	Log            logr.Logger
	WorkflowClient clientInterface.WorkflowClient
	ConfigProvider uberconfig.Provider
}

// NewBatchRerunTrigger creates a new batch rerun trigger Runner.
func NewBatchRerunTrigger(log logr.Logger, workflowClient clientInterface.WorkflowClient, configProvider uberconfig.Provider) Runner {
	return &batchRerunTrigger{
		Log:            log,
		WorkflowClient: workflowClient,
		ConfigProvider: configProvider,
	}
}

// Run starts a one-time batch rerun workflow. See backfillTrigger.Run for the shared
// start/idempotency pattern; the only difference is the workflow type.
func (r *batchRerunTrigger) Run(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	wid := generateWorkflowID(triggerRun)
	opt := clientInterface.StartWorkflowOptions{
		ID:                              wid,
		TaskList:                        "trigger_run",
		ExecutionStartToCloseTimeout:    time.Hour * 24 * 365, // 1 year, practically no timeout
		DecisionTaskStartToCloseTimeout: 30 * time.Second,
	}
	domain := r.WorkflowClient.GetDomain()
	rid, err := getWorkflowOpenRunID(ctx, wid, r.WorkflowClient, domain)
	if err != nil {
		// Don't return error - continue to attempt StartWorkflow, same as backfill.
		log.Error(err, "failed to get open workflow execution",
			"operation", "get_workflow_runid",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid)
	}
	if rid != nil && *rid != "" {
		log.Info("batch rerun cadence workflow already running",
			"operation", "run_batch_rerun_trigger",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", wid,
			"runId", *rid)
		return v2pb.TriggerRunStatus{
			State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
			ExecutionWorkflowId: *rid,
			LogUrl:              getWorkflowURL(r.ConfigProvider, wid, *rid),
		}, nil
	}
	log.Info("starting batch rerun workflow",
		"operation", "start_workflow",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"workflowId", opt.ID,
		"taskList", opt.TaskList)
	exec, err := r.WorkflowClient.StartWorkflow(
		ctx, opt, "trigger.BatchRerunTrigger", CreateTriggerRequest{TriggerRun: triggerRun})
	if err != nil {
		log.Error(err, "failed to start batch rerun workflow",
			"operation", "start_workflow",
			"namespace", triggerRun.Namespace,
			"name", triggerRun.Name,
			"workflowId", opt.ID)
		return v2pb.TriggerRunStatus{
				ErrorMessage: err.Error(),
				State:        v2pb.TRIGGER_RUN_STATE_FAILED,
			}, fmt.Errorf("start workflow for batch rerun trigger %s/%s: %w",
				triggerRun.Namespace, triggerRun.Name, err)
	}
	r.Log.Info("batch rerun workflow enabled",
		"operation", "workflow_started",
		"namespace", triggerRun.Namespace,
		"name", triggerRun.Name,
		"execution_id", exec.ID,
		"run_id", exec.RunID)
	return v2pb.TriggerRunStatus{
		State:               v2pb.TRIGGER_RUN_STATE_RUNNING,
		ExecutionWorkflowId: exec.ID,
		LogUrl:              getWorkflowURL(r.ConfigProvider, wid, exec.RunID),
	}, nil
}

// Kill terminates a running batch rerun workflow. See backfillTrigger.Kill.
func (r *batchRerunTrigger) Kill(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	if triggerRun.Status.State != v2pb.TRIGGER_RUN_STATE_RUNNING {
		err := fmt.Errorf("cannot kill batch rerun trigger run in state: %s", &triggerRun.Status.State)
		log.Error(err, "kill batch rerun trigger run failed")
		return v2pb.TriggerRunStatus{
			State:        triggerRun.Status.State,
			ErrorMessage: err.Error(),
		}, err
	}
	return killWorkflow(ctx, triggerRun, log, r.WorkflowClient)
}

// GetStatus retrieves the execution status of a batch rerun workflow. See
// backfillTrigger.GetStatus; both are one-time workflows and share the same status mapping.
func (r *batchRerunTrigger) GetStatus(
	ctx context.Context, triggerRun *v2pb.TriggerRun,
) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	domain := r.WorkflowClient.GetDomain()
	return getAdhocRunWorkflowStatus(ctx, triggerRun, log, r.WorkflowClient, domain)
}

// Pause is not supported for batch rerun triggers as they are one-time workflows.
func (r *batchRerunTrigger) Pause(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	err := fmt.Errorf("pause operation not supported for batch rerun triggers")
	log.Info("pause not supported for batch rerun trigger type")
	return v2pb.TriggerRunStatus{
		State:        v2pb.TRIGGER_RUN_STATE_FAILED,
		ErrorMessage: err.Error(),
	}, err
}

// Resume is not supported for batch rerun triggers as they are one-time workflows.
func (r *batchRerunTrigger) Resume(ctx context.Context, triggerRun *v2pb.TriggerRun) (v2pb.TriggerRunStatus, error) {
	log := r.Log.WithValues("triggerRun", k8stypes.NamespacedName{
		Namespace: triggerRun.Namespace,
		Name:      triggerRun.Name,
	})
	err := fmt.Errorf("resume operation not supported for batch rerun triggers")
	log.Info("resume not supported for batch rerun trigger type")
	return v2pb.TriggerRunStatus{
		State:        v2pb.TRIGGER_RUN_STATE_FAILED,
		ErrorMessage: err.Error(),
	}, err
}

// Update is a no-op for batch rerun triggers as they are one-time workflows.
func (r *batchRerunTrigger) Update(ctx context.Context, triggerRun *v2pb.TriggerRun, action v2pb.TriggerRunAction) (v2pb.TriggerRunStatus, bool, error) {
	return triggerRun.Status, false, nil
}
