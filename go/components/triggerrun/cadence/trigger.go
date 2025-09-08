package cadence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/components/utils"
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/cadence/activity"
	"go.uber.org/cadence/worker"
	"go.uber.org/cadence/workflow"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type contextKey int

const (
	contextKeyTriggerContext contextKey = iota
	contextKeylogicalTs

	// TriggerType constants for different trigger types
	TriggerTypeCron       = "cron"
	TriggerTypeBackfill   = "backfill"
	TriggerTypeBatchRerun = "batch_rerun"
	TriggerTypeInterval   = "interval"
	TriggerTypeUnknown    = "unknown"
)

var (
	//_uapiService = (*uapi.Service)(nil) // Used to execute UAPI Cadence activities
	_defaultParameterID     = "default"
	_defaultBatchSize       = 10
	_defaultWaitMinutes     = 10
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

	// TriggerredByLabel stores the name of the TriggerRun which triggered the pipeline_run
	TriggerredByLabel = "pipelinerun.michelangelo/triggered-by"

	// SourceTriggerLabel stores the original trigger associated with the pipeline_run.
	// For resume run, source-trigger is copied over from previous run
	SourceTriggerLabel = "pipelinerun.michelangelo/source-trigger"

	// PipelineRunExecutionTimestampLabel stores the execution timestamp of the pipeline run
	PipelineRunExecutionTimestampLabel = "pipelinerun.michelangelo/execution-timestamp"

	// EnvironmentLabel stores the environment of the pipeline run
	EnvironmentLabel = "michelangelo/environment"

	// PipelineManifestTypeLabel stores the type of the pipeline manifest
	PipelineManifestTypeLabel = "pipeline.michelangelo/PipelineManifestType"

	// NonRetriableErrorReasonsDefault defines errors that should not be retried
	NonRetriableErrorReasonsDefault = []string{
		"400",
		"404",
		"500",
		"cadenceInternal:Panic",
		"cadenceInternal:CanceledError",
		"no-retry",
	}

	SensorRetryPolicyDefault = workflow.RetryPolicy{
		InitialInterval:          20 * time.Second,
		BackoffCoefficient:       1,
		ExpirationInterval:       time.Hour * 24 * 14,
		NonRetriableErrorReasons: NonRetriableErrorReasonsDefault,
	}
)

type (
	// Object alias for map[string]interface{}
	Object = map[string]interface{}

	// Service contains all required workflows and activities for Cadence trigger
	Service struct {
		PipelineRunService v2pb.PipelineRunServiceYARPCClient
	}

	// CronTriggerRequest DTO for the CronTrigger workflow
	CronTriggerRequest struct {
		TriggerRun *v2pb.TriggerRun
	}

	// Params is for batch/concurrent run to store parameters for cron, backfill, batch rerun trigger
	Params struct {
		ParamID string
	}
)

// CronTrigger Cadence workflow to create PipelineRuns with provided trigger run spec
func (r *Service) CronTrigger(ctx workflow.Context, req CronTriggerRequest) (map[string]any, error) {
	ctx = workflow.WithActivityOptions(ctx, _activityOptionsDefault)
	tr := req.TriggerRun
	log := workflow.GetLogger(ctx).With(
		zap.Any("triggerRun", types.NamespacedName{Namespace: tr.Namespace, Name: tr.Name}),
	)
	logicalTs := workflow.Now(ctx).UTC()
	ctx = workflow.WithValue(ctx, contextKeylogicalTs, logicalTs)
	triggerContext := Object{
		// logical datestr of the triggered run, TODO: get DS from scheduler
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
	log.Info("starting cadence cron trigger", zap.Any("request", req))

	var err error
	if tr.Spec.Trigger.MaxConcurrency > 0 {
		err = r.concurrentRun(ctx, tr)
	} else {
		err = r.batchRun(ctx, tr)
	}
	if err != nil {
		return nil, err
	}
	triggerContext["FinishedAt"] = workflow.Now(ctx)
	return triggerContext, nil
}

func (r *Service) batchRun(ctx workflow.Context, tr *v2pb.TriggerRun) error {
	log := workflow.GetLogger(ctx).With(zap.String("namespace", tr.Namespace), zap.String("trigger_name", tr.Name))
	waitMinutes := _defaultWaitMinutes
	if tr.Spec.Trigger.BatchPolicy != nil && tr.Spec.Trigger.BatchPolicy.Wait != nil {
		waitMinutes = int(tr.Spec.Trigger.BatchPolicy.Wait.Seconds / 60) // Convert seconds to minutes
	}
	if len(tr.Spec.Trigger.ParametersMap) == 0 {
		tr.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
			_defaultParameterID: {},
		}
	}
	var batches [][]Params
	if err := workflow.ExecuteActivity(ctx, r.GenerateBatchRunParams, tr).Get(ctx, &batches); err != nil {
		log.Error("generate batch run parameters failed", zap.Error(err))
		return err
	}
	var err error
	// Each id in the trigger ParametersMap triggers one PipelineRun
	for idx, batch := range batches {
		for _, param := range batch {
			if _err := r.runPipeline(ctx, tr, param, false); _err != nil {
				log.Error("failed to run pipeline in trigger", zap.Error(_err), zap.Any("param_id", param))
				err = _err
			}
		}
		// wait for a period of time if not the last batch
		if idx < len(batches)-1 {
			workflow.Sleep(ctx, time.Minute*time.Duration(waitMinutes))
		}
	}
	return err
}

func (r *Service) concurrentRun(ctx workflow.Context, tr *v2pb.TriggerRun) error {
	logger := workflow.GetLogger(ctx)
	if len(tr.Spec.Trigger.ParametersMap) == 0 {
		tr.Spec.Trigger.ParametersMap = map[string]*v2pb.PipelineExecutionParameters{
			_defaultParameterID: {},
		}
	}
	var params []Params
	if err := workflow.ExecuteActivity(ctx, r.GenerateConcurrentRunParams, tr).Get(ctx, &params); err != nil {
		logger.Error("generate concurrent run parameters failed", zap.Error(err))
		return err
	}
	t := len(params)                         // Total params
	n := int(tr.Spec.Trigger.MaxConcurrency) // Initial batch size
	if t < n {
		n = t
	}

	selector := workflow.NewSelector(ctx)
	var err error

	// This function is executed upon future completion. It collects future's error, if any.
	futureF := func(f workflow.Future) {
		var v any
		if _err := f.Get(ctx, &v); _err != nil {
			logger.Error("Future.Get error", zap.Error(err))
			err = _err
		}
	}

	// Run initial N params all at once
	for i := 0; i < n; i++ {
		param := params[i]
		logger.Info("run pipeline async", zap.Any("current parameter", param), zap.Int("param_idx", i))
		f := r.runPipelineAsync(ctx, tr, param, true)
		selector.AddFuture(f, futureF)
	}
	// Run rest of the params gradually
	for i := 0; i < t; i++ {
		selector.Select(ctx)
		if n < t {
			param := params[n]
			logger.Info("run pipeline async", zap.Any("current parameter", param), zap.Int("param_idx", n))
			f := r.runPipelineAsync(ctx, tr, param, true)
			selector.AddFuture(f, futureF)
			n++
		}
	}
	return err
}

// GenerateBatchRunParams activity generates batch triggered run parameters in [][]Params
func (r *Service) GenerateBatchRunParams(ctx context.Context, tr *v2pb.TriggerRun) ([][]Params, error) {
	triggerType := GetTriggerType(tr)
	var (
		params [][]Params
		err    error
	)
	switch triggerType {
	case TriggerTypeCron, TriggerTypeInterval:
		params, err = generateBatchCronParams(tr)
	default:
		return nil, fmt.Errorf("generate batch run parameters failed")
	}
	return params, err
}

// GenerateConcurrentRunParams activity generates concurrent triggered run parameters in []Params
func (r *Service) GenerateConcurrentRunParams(ctx context.Context, tr *v2pb.TriggerRun) ([]Params, error) {
	triggerType := GetTriggerType(tr)
	var (
		params []Params
		err    error
	)
	switch triggerType {
	case TriggerTypeCron, TriggerTypeInterval:
		params, err = generateConcurrentCronParams(tr)
	default:
		return nil, fmt.Errorf("generate concurrent run parameters failed")
	}
	return params, err
}

// generateConcurrentCronParams function generates cron triggered run parameters in []Params
func generateConcurrentCronParams(triggerRun *v2pb.TriggerRun) ([]Params, error) {
	params := make([]Params, len(triggerRun.Spec.Trigger.ParametersMap))
	i := 0
	for paramID := range triggerRun.Spec.Trigger.ParametersMap {
		params[i] = Params{
			ParamID: paramID,
		}
		i++
	}
	sortParams(params)
	return params, nil
}

func (r *Service) runPipeline(ctx workflow.Context, triggerRun *v2pb.TriggerRun, param Params, sensor bool) error {
	log := workflow.GetLogger(ctx)
	triggerContext := ctx.Value(contextKeyTriggerContext).(Object)
	name := generatePipelineRunName(workflow.Now(ctx))
	var (
		createRequest v2pb.CreatePipelineRunRequest
		err           error
		pr            *v2pb.PipelineRun
	)
	triggeredRuns := triggerContext["TriggeredRuns"]
	switch triggeredRuns.(type) {
	case map[string]Object:
		logicalTs := ctx.Value(contextKeylogicalTs).(time.Time)
		createRequest, err = generatePipelineRunRequest(triggerRun, param.ParamID, name, logicalTs)
	default:
		err = fmt.Errorf("invalid type for TriggeredRuns: %T", triggeredRuns)
	}
	if err != nil {
		log.Error("failed to generate CreatePipelineRunRequest", zap.Error(err))
		return err
	}
	if err = workflow.ExecuteActivity(ctx, r.CreatePipelineRun, createRequest).Get(ctx, &pr); err != nil {
		log.Error("CreatePipelineRun error", zap.Any("parameter", param), zap.Error(err), zap.Any("request", createRequest))
		return err
	}
	log.Info("pipeline run created", zap.Any("parameter", param), zap.Any("pipeline_run", pr))
	createdTimestamp := workflow.Now(ctx)
	switch triggeredRuns.(type) {
	case map[string]Object:
		// TriggeredRuns is a map[parameter_id -> run_information]
		triggerContext["TriggeredRuns"].(map[string]Object)[param.ParamID] = Object{
			"PipelineRunName": pr.Name,
			"CreatedAt":       createdTimestamp,
		}
	}
	if !sensor {
		return nil
	}
	sensorRequest := v2pb.GetPipelineRunRequest{
		Namespace: pr.Namespace,
		Name:      pr.Name,
	}
	sensorCtx := workflow.WithRetryPolicy(ctx, SensorRetryPolicyDefault)
	if err = workflow.ExecuteActivity(sensorCtx, r.PipelineRunSensor, sensorRequest).Get(sensorCtx, &pr); err != nil {
		log.Error("PipelineRunSensor error", zap.Any("current parameter", param), zap.Error(err), zap.Any("request", sensorRequest))
		return err
	}
	return nil
}

func (r *Service) runPipelineAsync(ctx workflow.Context, triggerRun *v2pb.TriggerRun, param Params, sensor bool) workflow.Future {
	future, settable := workflow.NewFuture(ctx)
	workflow.Go(ctx, func(ctx workflow.Context) {
		err := r.runPipeline(ctx, triggerRun, param, sensor)
		settable.SetError(err)
	})
	return future
}

// CreatePipelineRun activity
func (r *Service) CreatePipelineRun(ctx context.Context, req v2pb.CreatePipelineRunRequest) (*v2pb.PipelineRun, error) {
	res, err := r.PipelineRunService.CreatePipelineRun(ctx, &req)
	if err != nil {
		return nil, err
	}
	return res.PipelineRun, err
}

// PipelineRunSensor activity
func (r *Service) PipelineRunSensor(
	ctx context.Context,
	req v2pb.GetPipelineRunRequest,
) (*v2pb.PipelineRun, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("activity", zap.Any("namespace", req.Namespace), zap.Any("name", req.Name))

	res, err := r.PipelineRunService.GetPipelineRun(ctx, &req)
	if err != nil {
		logger.Error(fmt.Sprintf("%T: %s", err, err.Error()), zap.Error(err))
		return nil, fmt.Errorf("upstream error: %w", err)
	}
	res.PipelineRun = cropPipelineRun(res.PipelineRun)
	switch res.PipelineRun.Status.State {
	case
		v2pb.PIPELINE_RUN_STATE_INVALID,
		v2pb.PIPELINE_RUN_STATE_PENDING,
		v2pb.PIPELINE_RUN_STATE_RUNNING:
		return nil, fmt.Errorf("pipeline_run terminated with state: %v", res.PipelineRun.Status.State)
	}
	return res.PipelineRun, nil
}

// cropPipelineRun function crops the pipeline run to remove unnecessary fields
func cropPipelineRun(r *v2pb.PipelineRun) *v2pb.PipelineRun {
	if r == nil {
		return nil
	}
	status := r.Status
	res := &v2pb.PipelineRun{
		TypeMeta: r.TypeMeta,
		ObjectMeta: v1.ObjectMeta{
			Namespace:   r.Namespace,
			Name:        r.Name,
			Labels:      r.Labels,
			Annotations: r.Annotations,
		},
		Spec: r.Spec,
		Status: v2pb.PipelineRunStatus{
			State:        status.State,
			LogUrl:       status.LogUrl,
			ErrorMessage: status.ErrorMessage,
			Code:         status.Code,
			EndTime:      status.EndTime,
		},
	}
	return res
}

// generateBatchCronParams function generates cron triggered run parameters in [][]Params
func generateBatchCronParams(triggerRun *v2pb.TriggerRun) ([][]Params, error) {
	batchSize := _defaultBatchSize
	if triggerRun.Spec.Trigger.BatchPolicy != nil && triggerRun.Spec.Trigger.BatchPolicy.BatchSize != 0 {
		batchSize = int(triggerRun.Spec.Trigger.BatchPolicy.BatchSize)
	}
	paramsMap := triggerRun.Spec.Trigger.ParametersMap
	numOfBatches := 1
	if len(paramsMap) > 0 {
		if len(paramsMap)%batchSize == 0 {
			numOfBatches = len(paramsMap) / batchSize
		} else {
			numOfBatches = len(paramsMap)/batchSize + 1
		}
	}

	batchedParams := make([][]Params, numOfBatches)

	// no parameters are defined for this trigger
	if len(paramsMap) == 0 {
		batchedParams[0] = []Params{{ParamID: ""}}
		return batchedParams, nil
	}
	keys := make([]Params, 0, len(paramsMap))
	for k := range paramsMap {
		cur := Params{
			ParamID: k,
		}
		keys = append(keys, cur)
	}
	sortParams(keys)
	for i := 0; i < len(keys); i = i + batchSize {
		if i+batchSize <= len(keys) {
			batchedParams[i/batchSize] = keys[i : i+batchSize]
		} else {
			batchedParams[i/batchSize] = keys[i:]
		}
	}
	return batchedParams, nil
}

// sort the parameters in Param Struct to make cron, backfill and batch rerun trigger deterministic
func sortParams(params []Params) {
	sort.Slice(params, func(i, j int) bool {
		return params[i].ParamID < params[j].ParamID
	})
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
	}
	var (
		pbStruct *pbtypes.Struct
		err      error
	)
	paramsMap := triggerRun.Spec.Trigger.ParametersMap
	if len(paramsMap) > 0 {
		p, ok := paramsMap[paramID]
		if ok {
			labels["pipelinerun.michelangelo/parameter-id"] = paramID
			if triggerRun.Labels[PipelineManifestTypeLabel] == "PIPELINE_MANIFEST_TYPE_UNIFLOW" {
				pbStruct = generateUniflowPRInput(p)
			} else {
				parameters := map[string]interface{}{}
				for k, v := range p.ParameterMap {
					parameters[k] = v
				}
				pbStruct, err = utils.NewStruct(parameters)
			}
			if err != nil {
				return v2pb.CreatePipelineRunRequest{}, err
			}
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

		// Load KwArgs to the pipeline run input - order kw_args for deterministic behavior, so cadence workflow runs are idempotent
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

// GetTriggerType returns the trigger type for a given triggerRun
func GetTriggerType(tr *v2pb.TriggerRun) string {
	if tr.Spec.Trigger.GetBatchRerun() != nil {
		return TriggerTypeBatchRerun
	}
	if tr.Spec.StartTimestamp != nil && tr.Spec.EndTimestamp != nil {
		return TriggerTypeBackfill
	}
	if tr.Spec.Trigger.GetIntervalSchedule() != nil {
		return TriggerTypeInterval
	}
	if tr.Spec.Trigger.GetCronSchedule() != nil {
		return TriggerTypeCron
	}
	return TriggerTypeUnknown
}

// GenerateRandomString generates a random hex string of specified length
func generateRandomString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("%d", time.Now().UnixNano())[:length]
	}
	return hex.EncodeToString(bytes)[:length]
}

// GeneratePipelineRunName generates a unique name for a pipeline run based on timestamp
func generatePipelineRunName(t time.Time) string {
	// Generate random string for uniqueness
	randomStr := generateRandomString(8)
	// Format: pipeline-run-YYYYMMDD-HHMMSS-RANDOM
	return fmt.Sprintf("pipeline-run-%s-%s", t.Format("20060102-150405"), randomStr)
}

// Register - register Service's activities and workflows.
func Register(service *Service, ns string, reg worker.Registry) {
	reg.RegisterActivityWithOptions(service.CreatePipelineRun, activity.RegisterOptions{
		Name: fmt.Sprintf("%s.%s", ns, "CreatePipelineRun"),
	})

	reg.RegisterActivityWithOptions(service.GenerateBatchRunParams, activity.RegisterOptions{
		Name: fmt.Sprintf("%s.%s", ns, "GenerateBatchRunParams"),
	})

	reg.RegisterActivityWithOptions(service.GenerateConcurrentRunParams, activity.RegisterOptions{
		Name: fmt.Sprintf("%s.%s", ns, "GenerateConcurrentRunParams"),
	})

	reg.RegisterActivityWithOptions(service.PipelineRunSensor, activity.RegisterOptions{
		Name: fmt.Sprintf("%s.%s", ns, "PipelineRunSensor"),
	})

	reg.RegisterWorkflowWithOptions(service.CronTrigger, workflow.RegisterOptions{
		Name: fmt.Sprintf("%s.%s", ns, "CronTrigger"),
	})
}
