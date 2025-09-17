package trigger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	triggerrunUtil "github.com/michelangelo-ai/michelangelo/go/components/triggerrun"
	"github.com/michelangelo-ai/michelangelo/go/components/utils"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
	"github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger/parameter"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var Workflows = (*workflows)(nil)

type (
	// workflows struct encapsulates the trigger workflow
	workflows struct{}

	// Object alias for map[string]interface{}
	Object = map[string]interface{}
)

const (
	contextKeyTriggerContext = iota
	contextKeylogicalTs
)

var (
	// _defaultWaitSeconds is the default wait seconds for the trigger workflow
	_defaultWaitSeconds = 600

	// _defaultParameterID is the default parameter id for the trigger workflow
	_defaultParameterID = "default"

	// TriggerredByLabel stores the name of the TriggerRun which triggered the pipeline_run
	TriggerredByLabel = "pipelinerun.michelangelo/triggered-by"

	// EnvironmentLabel stores the environment of the pipeline_run
	EnvironmentLabel = "pipelinerun.michelangelo/environment"

	// SourceTriggerLabel stores the original trigger associated with the pipeline_run.
	// For resume run, source-trigger is copied over from previous run
	SourceTriggerLabel = "pipelinerun.michelangelo/source-trigger"

	// PipelineRunExecutionTimestampLabel is used to record the logic execution timestamp of the pipeline run in RFC3339 format
	PipelineRunExecutionTimestampLabel = "pipelinerun.michelangelo/execution-timestamp"

	// ParameterIDLabel is used to record the parameter id of the pipeline run
	ParameterIDLabel = "pipelinerun.michelangelo/parameter-id"

	// PipelineManifestTypeLabel is to indicate the manifest type of this pipeline
	PipelineManifestTypeLabel = "pipeline.michelangelo/PipelineManifestType"

	// _activityOptionsDefault is the default activity options for the trigger workflow
	_activityOptionsDefault = workflow.ActivityOptions{
		ScheduleToStartTimeout: time.Second * 30,
		StartToCloseTimeout:    time.Second * 30,
		RetryPolicy: &workflow.RetryPolicy{
			InitialInterval:          time.Millisecond * 500,
			BackoffCoefficient:       2.0,
			MaximumAttempts:          5,
			NonRetriableErrorReasons: NonRetriableErrorReasonsDefault,
		},
	}

	// NonRetriableErrorReasonsDefault defines errors that should not be retried
	NonRetriableErrorReasonsDefault = []string{
		"400",
		"404",
		"500",
		"cadenceInternal:Panic",
		"cadenceInternal:CanceledError",
		"no-retry",
	}

	// SensorRetryPolicyDefault is the default retry policy for the sensor activity
	SensorRetryPolicyDefault = workflow.RetryPolicy{
		InitialInterval:          20 * time.Second,
		BackoffCoefficient:       1,
		ExpirationInterval:       time.Hour * 24 * 14,
		NonRetriableErrorReasons: NonRetriableErrorReasonsDefault,
	}
)

// CronTrigger workflow with provided trigger run spec
func (r *workflows) CronTrigger(ctx workflow.Context, req triggerrunUtil.CreateTriggerRequest) (map[string]any, error) {
	ctx = workflow.WithActivityOptions(ctx, _activityOptionsDefault)
	tr := req.TriggerRun
	log := workflow.GetLogger(ctx).With(
		zap.Any("triggerRun", types.NamespacedName{Namespace: tr.Namespace, Name: tr.Name}),
	)
	logicalTs := workflow.Now(ctx).UTC()
	ctx = workflow.WithValue(ctx, contextKeylogicalTs, logicalTs)
	triggerContext := Object{
		"DS":            logicalTs.Format("2006-01-02"),
		"StartedAt":     workflow.Now(ctx),
		"TriggeredRuns": map[string]Object{},
	}
	ctx = workflow.WithValue(ctx, contextKeyTriggerContext, triggerContext)
	// setup query handler for runHistory
	if err := workflow.SetQueryHandler(ctx, "triggerContext", func() (map[string]any, error) {
		return triggerContext, nil
	}); err != nil {
		log.Error("setQueryHandler for triggerContext failed", zap.Error(err))
		return nil, err
	}
	log.Info("starting cron trigger workflow", zap.Any("request", req))

	// Use shared package functions
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

// batchRun executes trigger runs in batches with configurable wait times between batches
func batchRun(ctx workflow.Context, tr *v2pb.TriggerRun) error {
	log := workflow.GetLogger(ctx).With(zap.String("namespace", tr.Namespace), zap.String("trigger_name", tr.Name))
	waitSeconds := _defaultWaitSeconds
	if tr.Spec.Trigger.BatchPolicy != nil && tr.Spec.Trigger.BatchPolicy.Wait != nil {
		waitSeconds = int(tr.Spec.Trigger.BatchPolicy.Wait.Seconds)
	}
	if len(tr.Spec.Trigger.ParametersMap) == 0 {
		tr.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
			_defaultParameterID: {},
		}
	}
	var (
		batches [][]parameter.Params
		err     error
	)
	if err = workflow.ExecuteActivity(ctx, trigger.Activities.GenerateBatchRunParams, tr).Get(ctx, &batches); err != nil {
		log.Error("GenerateBatchRunParams failed", zap.Error(err))
		return err
	}
	for idx, batch := range batches {
		for _, param := range batch {
			if _err := runPipeline(ctx, tr, param, false); _err != nil {
				log.Error("failed to run pipeline in trigger", zap.Error(_err), zap.Any("param_id", param))
				err = _err
			}
		}
		// wait for a period of time if not the last batch
		if idx < len(batches)-1 {
			workflow.Sleep(ctx, time.Second*time.Duration(waitSeconds))
		}
	}
	return nil
}

// concurrentRun executes trigger runs concurrently with configurable max concurrency
func concurrentRun(ctx workflow.Context, tr *v2pb.TriggerRun) error {
	log := workflow.GetLogger(ctx)
	if len(tr.Spec.Trigger.ParametersMap) == 0 {
		tr.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
			_defaultParameterID: {},
		}
	}
	var (
		params []parameter.Params
		err    error
	)
	if err = workflow.ExecuteActivity(ctx, trigger.Activities.GenerateConcurrentRunParams, tr).Get(ctx, &params); err != nil {
		log.Error("generate concurrent run parameters failed", zap.Error(err))
		return err
	}
	t := len(params)
	n := int(tr.Spec.Trigger.MaxConcurrency)
	if t < n {
		n = t
	}
	selector := workflow.NewSelector(ctx)
	// This function is executed upon future completion. It collects future's error, if any.
	futureF := func(f workflow.Future) {
		var v any
		if _err := f.Get(ctx, &v); _err != nil {
			log.Error("Future.Get error", zap.Error(_err))
			err = _err
		}
	}
	// Run initial N params all at once
	for i := 0; i < n; i++ {
		param := params[i]
		log.Info("run pipeline async", zap.Any("current parameter", param), zap.Int("param_idx", i))
		f := runPipelineAsync(ctx, tr, param, true)
		selector.AddFuture(f, futureF)
	}
	// Run rest of the params gradually
	for i := 0; i < t; i++ {
		selector.Select(ctx)
		if n < t {
			param := params[n]
			log.Info("run pipeline async", zap.Any("current parameter", param), zap.Int("param_idx", n))
			f := runPipelineAsync(ctx, tr, param, true)
			selector.AddFuture(f, futureF)
			n++
		}
	}
	return err
}

// runPipeline creates and optionally monitors a pipeline run
func runPipeline(ctx workflow.Context, triggerRun *v2pb.TriggerRun, param parameter.Params, sensor bool) error {
	log := workflow.GetLogger(ctx)
	name := generatePipelineRunName(workflow.Now(ctx))
	var (
		createRequest v2pb.CreatePipelineRunRequest
		err           error
		pr            *v2pb.PipelineRun
	)
	triggerContext := ctx.Value(contextKeyTriggerContext).(Object)
	logicalTs := ctx.Value(contextKeylogicalTs).(time.Time)
	createRequest, err = generatePipelineRunRequest(triggerRun, param.ParamID, name, logicalTs)
	if err != nil {
		log.Error("failed to generate CreatePipelineRunRequest", zap.Error(err))
		return err
	}
	if err = workflow.ExecuteActivity(ctx, trigger.Activities.CreatePipelineRun, createRequest).Get(ctx, &pr); err != nil {
		log.Error("CreatePipelineRun error", zap.Any("parameter", param), zap.Error(err), zap.Any("request", createRequest))
		return err
	}
	log.Info("pipeline run created", zap.Any("parameter", param), zap.Any("pipeline_run", pr))
	createdTimestamp := workflow.Now(ctx)
	// TriggeredRuns is a map[parameter_id -> run_information]
	triggerContext["TriggeredRuns"].(map[string]Object)[param.ParamID] = Object{
		"PipelineRunName": pr.Name,
		"CreatedAt":       createdTimestamp,
	}
	if !sensor {
		return nil
	}
	sensorRequest := v2pb.GetPipelineRunRequest{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}
	sensorCtx := workflow.WithRetryPolicy(ctx, SensorRetryPolicyDefault)
	if err := workflow.ExecuteActivity(sensorCtx, trigger.Activities.PipelineRunSensor, sensorRequest).Get(sensorCtx, &pr); err != nil {
		log.Error("PipelineRunSensor failed", zap.Error(err))
		return err
	}
	return nil
}

// runPipelineAsync runs a pipeline asynchronously and returns a Future
func runPipelineAsync(ctx workflow.Context, triggerRun *v2pb.TriggerRun, param parameter.Params, sensor bool) workflow.Future {
	future, settable := workflow.NewFuture(ctx)
	workflow.Go(ctx, func(ctx workflow.Context) {
		err := runPipeline(ctx, triggerRun, param, sensor)
		settable.SetError(err)
	})
	return future
}

func generatePipelineRunRequest(
	triggerRun *v2pb.TriggerRun, paramID string, pipelineRunName string, ts time.Time,
) (v2pb.CreatePipelineRunRequest, error) {
	labels := map[string]string{
		PipelineRunExecutionTimestampLabel: fmt.Sprintf("%d", ts.Unix()),
		TriggerredByLabel:                  triggerRun.Name,
		SourceTriggerLabel:                 triggerRun.Name,
	}
	if env, ok := triggerRun.ObjectMeta.Labels[EnvironmentLabel]; ok {
		labels[EnvironmentLabel] = env
	} else {
		labels[EnvironmentLabel] = "production"
	}
	annotations := map[string]string{
		"michelangelo.uber.com/pipelinerun.engine": "condition",
		"michelangelo/UpdateTimestamp":             fmt.Sprintf("%d", ts.Unix()),
		"michelangelo/SpecUpdateTimestamp":         fmt.Sprintf("%d", ts.Unix()),
	}
	var pbStruct *pbtypes.Struct
	paramsMap := triggerRun.Spec.Trigger.ParametersMap
	if len(paramsMap) > 0 {
		p, ok := paramsMap[paramID]
		if ok {
			labels[ParameterIDLabel] = paramID
			pbStruct = generateUniflowPRInput(p)
		} else {
			return v2pb.CreatePipelineRunRequest{}, fmt.Errorf("invalid parameter id: %s", paramID)
		}
	}
	pr := &v2pb.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			Name:        pipelineRunName,
			Namespace:   triggerRun.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v2pb.PipelineRunSpec{
			Pipeline: &api.ResourceIdentifier{
				Namespace: triggerRun.Spec.Pipeline.Namespace,
				Name:      triggerRun.Spec.Pipeline.Name,
			},
			Input: pbStruct,
		},
	}
	if triggerRun.Spec.Actor != nil {
		pr.Spec.Actor = &v2pb.UserInfo{
			Name: triggerRun.Spec.Actor.Name,
		}
	}
	if triggerRun.Spec.Revision != nil {
		pr.Spec.Revision = &api.ResourceIdentifier{
			Namespace: triggerRun.Spec.Revision.Namespace,
			Name:      triggerRun.Spec.Revision.Name,
		}
	}
	pr.Labels[PipelineManifestTypeLabel] = triggerRun.Labels[PipelineManifestTypeLabel]
	return v2pb.CreatePipelineRunRequest{
		PipelineRun: pr,
	}, nil
}

func generateUniflowPRInput(dp *v2pb.PipelineExecutionParameters) *pbtypes.Struct {
	pbStruct := &pbtypes.Struct{
		Fields: make(map[string]*pbtypes.Value),
	}
	switch {
	// Generate the input for Canvas flex PipelineRun
	case dp.WorkflowConfig != nil || dp.TaskConfigs != nil:
		// Load WorkflowConfig to the pipeline run input
		pbStruct.Fields["workflow_config"] = utils.NewStructValue(dp.WorkflowConfig)

		// Load TaskConfigs to the pipeline run input
		taskStruct := &pbtypes.Struct{Fields: make(map[string]*pbtypes.Value)}
		if dp.TaskConfigs != nil {
			for k, v := range dp.TaskConfigs {
				taskStruct.Fields[k] = utils.NewStructValue(v)
			}
		}
		pbStruct.Fields["task_configs"] = utils.NewStructValue(taskStruct)

	// Generate the input for Uniflow PipelineRun
	case dp.Environ != nil || dp.KwArgs != nil || dp.Args != nil:
		// Load Environ to the pipeline run input
		envStruct := &pbtypes.Struct{Fields: make(map[string]*pbtypes.Value)}
		if dp.Environ != nil {
			for k, v := range dp.Environ {
				envStruct.Fields[k] = utils.NewStringValue(v)
			}
		}
		pbStruct.Fields["environ"] = utils.NewStructValue(envStruct)

		// Load Args to the pipeline run input
		argList := make([]*pbtypes.Value, 0)
		if dp.Args != nil {
			for _, argStruct := range dp.Args {
				argList = append(argList, utils.NewStructValue(argStruct))
			}
		}
		pbStruct.Fields["args"] = utils.NewListValue(&pbtypes.ListValue{Values: argList})

		// Load KwArgs to the pipeline run input - order kw_args for deterministic behavior, so workflow runs are idempotent
		keys := make([]string, 0)
		kwargList := make([]*pbtypes.Value, 0)
		if dp.KwArgs != nil && dp.KwArgs.Fields != nil {
			for k := range dp.KwArgs.Fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if val, exists := dp.KwArgs.Fields[key]; exists {
					kwargList = append(kwargList, utils.NewListValue(&pbtypes.ListValue{
						Values: []*pbtypes.Value{
							utils.NewStringValue(key),
							val,
						},
					}))
				}
			}
		}
		pbStruct.Fields["kwargs"] = utils.NewListValue(&pbtypes.ListValue{Values: kwargList})
	}
	return pbStruct
}

// generatePipelineRunName generates a unique name for a pipeline run based on timestamp
func generatePipelineRunName(t time.Time) string {
	// Generate random string for uniqueness
	randomStr := generateRandomString(8)
	// Format: pipeline-run-YYYYMMDD-HHMMSS-RANDOM
	return fmt.Sprintf("pipeline-run-%s-%s", t.Format("20060102-150405"), randomStr)
}

// generateRandomString generates a random hex string of specified length
func generateRandomString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
	}
	return hex.EncodeToString(bytes)[:length]
}
