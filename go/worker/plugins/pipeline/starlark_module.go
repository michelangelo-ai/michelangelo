package pipeline

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/cadence-workflow/starlark-worker/ext"
	"github.com/cadence-workflow/starlark-worker/service"
	"github.com/cadence-workflow/starlark-worker/workflow"
	pbtypes "github.com/gogo/protobuf/types"
	"go.starlark.net/starlark"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	computils "github.com/michelangelo-ai/michelangelo/go/components/utils"
	trigger "github.com/michelangelo-ai/michelangelo/go/worker/activities/trigger"
	"github.com/michelangelo-ai/michelangelo/go/worker/plugins/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

var _ starlark.HasAttrs = (*module)(nil)

type module struct {
	runInfo    service.RunInfo
	attributes map[string]starlark.Value
}

func newModule(runInfo service.RunInfo) starlark.Value {
	m := &module{
		runInfo: runInfo,
	}
	m.attributes = map[string]starlark.Value{
		"run_pipeline": starlark.NewBuiltin("run_pipeline", m.runPipeline).BindReceiver(m),
	}
	return m
}

func (r *module) String() string                        { return pluginID }
func (r *module) Type() string                          { return pluginID }
func (r *module) Freeze()                               {}
func (r *module) Truth() starlark.Bool                  { return true }
func (r *module) Hash() (uint32, error)                 { return 0, fmt.Errorf("no-hash") }
func (r *module) Attr(n string) (starlark.Value, error) { return r.attributes[n], nil }
func (r *module) AttrNames() []string                   { return ext.SortedKeys(r.attributes) }

// generatePipelineRunName creates a unique pipeline run name with timestamp and random suffix.
func generatePipelineRunName() (string, error) {
	now := time.Now()
	timestamp := now.Format("20060102-150405")

	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random suffix: %w", err)
	}
	suffix := hex.EncodeToString(bytes)

	return fmt.Sprintf("run-%s-%s", timestamp, suffix), nil
}

// runPipeline creates and waits for a child pipeline run to complete synchronously
//
//	run_pipeline(namespace, pipeline_name, pipeline_revision="", environ=None, args=None, kwargs=None, timeout_seconds=0, poll_seconds=10, input_data=None, actor=None) -> pipeline_run
//
//	  namespace: str: Namespace where the pipeline run will be created (required)
//	  pipeline_name: str: Name of the pipeline to run (required)
//	  pipeline_revision?: str: Optional git SHA specifying a particular pipeline version for reproducible runs
//	  environ?: dict: Optional dictionary of environment variables (map[string]string)
//	  args?: list: Optional list of pipeline-specific arguments
//	  kwargs?: dict: Optional dictionary of pipeline-specific keyword configurations
//	  timeout_seconds?: int: Maximum time in seconds to wait for completion (default: 0 = uses CadenceLongTimeout)
//	  poll_seconds?: int: Polling interval in seconds (default: 10)
//	  input_data?: dict: Optional input parameters for non-Uniflow pipelines (mutually exclusive with environ/args/kwargs)
//	  actor?: str: Optional name of the actor creating the pipeline run (default: None)
//
//	  return: dict: PipelineRun details with metadata (name, namespace) and status (state)
func (r *module) runPipeline(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	ctx := service.GetContext(t)
	logger := workflow.GetLogger(ctx)

	var namespace string
	var pipelineName string
	var pipelineRevision string
	var environDict *starlark.Dict
	var argsList *starlark.List
	var kwargsDict *starlark.Dict
	var timeoutSeconds int64 = 0
	var pollSeconds int64 = 10
	var inputDataDict *starlark.Dict
	var actor string

	if err := starlark.UnpackArgs("run_pipeline", args, kwargs,
		"namespace", &namespace,
		"pipeline_name", &pipelineName,
		"pipeline_revision?", &pipelineRevision,
		"environ?", &environDict,
		"args?", &argsList,
		"kwargs?", &kwargsDict,
		"timeout_seconds?", &timeoutSeconds,
		"poll_seconds?", &pollSeconds,
		"input_data?", &inputDataDict,
		"actor?", &actor,
	); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Check mutual exclusivity: input_data vs (environ/args/kwargs)
	hasInputData := inputDataDict != nil
	hasUniflowParams := environDict != nil || argsList != nil || kwargsDict != nil

	if hasInputData && hasUniflowParams {
		logger.Error("builtin-error", ext.ZapError(fmt.Errorf("input_data cannot be used together with environ, args, or kwargs"))...)
		return nil, fmt.Errorf("input_data cannot be used together with environ, args, or kwargs")
	}

	// Generate pipeline run name
	name, err := generatePipelineRunName()
	if err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Format revision if provided
	var revision *apipb.ResourceIdentifier
	if pipelineRevision != "" {
		// Format: pipeline-{name}-{sha[0:12]}
		shaPrefix := pipelineRevision
		if len(shaPrefix) > 12 {
			shaPrefix = shaPrefix[0:12]
		}
		formatted := fmt.Sprintf("pipeline-%s-%s", pipelineName, shaPrefix)
		revision = &apipb.ResourceIdentifier{
			Namespace: namespace,
			Name:      formatted,
		}
	}

	// Build input Struct based on provided parameters
	var pbStruct *pbtypes.Struct
	if hasInputData {
		// For non-Uniflow pipelines: use input_data directly
		if err := utils.AsGo(inputDataDict, &pbStruct); err != nil {
			logger.Error("builtin-error", ext.ZapError(err)...)
			return nil, err
		}
	} else if hasUniflowParams {
		// Build input Struct for Uniflow parameters (matching internal version structure)
		pbStruct = &pbtypes.Struct{
			Fields: make(map[string]*pbtypes.Value),
		}

		// Process environ (map[string]string)
		if environDict != nil {
			var environMap map[string]string
			if err := utils.AsGo(environDict, &environMap); err != nil {
				logger.Error("builtin-error", ext.ZapError(err)...)
				return nil, fmt.Errorf("failed to convert environ dict: %w", err)
			}
			envStruct := &pbtypes.Struct{Fields: make(map[string]*pbtypes.Value)}
			for k, v := range environMap {
				envStruct.Fields[k] = computils.NewStringValue(v)
			}
			pbStruct.Fields["environ"] = computils.NewStructValue(envStruct)
		}

		// Process args (list of Struct)
		if argsList != nil {
			var argsSlice []interface{}
			if err := utils.AsGo(argsList, &argsSlice); err != nil {
				logger.Error("builtin-error", ext.ZapError(err)...)
				return nil, fmt.Errorf("failed to convert args list: %w", err)
			}
			argList := make([]*pbtypes.Value, 0, len(argsSlice))
			for _, arg := range argsSlice {
				var argStruct *pbtypes.Struct
				if argMap, ok := arg.(map[string]interface{}); ok {
					var err error
					argStruct, err = computils.NewStruct(argMap)
					if err != nil {
						logger.Error("builtin-error", ext.ZapError(err)...)
						return nil, err
					}
				} else {
					argVal, err := computils.NewValue(arg)
					if err != nil {
						logger.Error("builtin-error", ext.ZapError(err)...)
						return nil, err
					}
					argStruct = &pbtypes.Struct{
						Fields: map[string]*pbtypes.Value{
							"value": argVal,
						},
					}
				}
				argList = append(argList, computils.NewStructValue(argStruct))
			}
			pbStruct.Fields["args"] = computils.NewListValue(&pbtypes.ListValue{Values: argList})
		}

		// Process kwargs (dict -> sorted list of [key, value] pairs)
		if kwargsDict != nil {
			var kwargsMap map[string]interface{}
			if err := utils.AsGo(kwargsDict, &kwargsMap); err != nil {
				logger.Error("builtin-error", ext.ZapError(err)...)
				return nil, fmt.Errorf("failed to convert kwargs dict: %w", err)
			}
			// Sort keys for deterministic behavior
			keys := make([]string, 0, len(kwargsMap))
			for k := range kwargsMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			kwargList := make([]*pbtypes.Value, 0, len(keys))
			for _, key := range keys {
				val := kwargsMap[key]
				var valProto *pbtypes.Value
				if valStruct, ok := val.(map[string]interface{}); ok {
					structVal, err := computils.NewStruct(valStruct)
					if err != nil {
						logger.Error("builtin-error", ext.ZapError(err)...)
						return nil, err
					}
					valProto = computils.NewStructValue(structVal)
				} else {
					var err error
					valProto, err = computils.NewValue(val)
					if err != nil {
						logger.Error("builtin-error", ext.ZapError(err)...)
						return nil, err
					}
				}
				kwargList = append(kwargList, computils.NewListValue(&pbtypes.ListValue{
					Values: []*pbtypes.Value{
						computils.NewStringValue(key),
						valProto,
					},
				}))
			}
			pbStruct.Fields["kwargs"] = computils.NewListValue(&pbtypes.ListValue{Values: kwargList})
		}
	}
	// Note: pbStruct may be nil if neither input_data nor Uniflow params are provided

	// Build CreatePipelineRunRequest
	request := &v2pb.CreatePipelineRunRequest{
		PipelineRun: &v2pb.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v2pb.PipelineRunSpec{
				Pipeline: &apipb.ResourceIdentifier{
					Namespace: namespace,
					Name:      pipelineName,
				},
			},
		},
		CreateOptions: &metav1.CreateOptions{},
	}

	// Set revision if provided
	if revision != nil {
		request.PipelineRun.Spec.Revision = revision
	}

	// Set actor if available
	if actor != "" {
		request.PipelineRun.Spec.Actor = &v2pb.UserInfo{
			Name: actor,
		}
	}

	// Set input Struct if provided
	if pbStruct != nil {
		request.PipelineRun.Spec.Input = pbStruct
	}

	// Execute CreatePipelineRun activity
	var pipelineRun *v2pb.PipelineRun
	if err := workflow.ExecuteActivity(ctx, trigger.Activities.CreatePipelineRun, request).Get(ctx, &pipelineRun); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Always wait for completion (synchronous by design)
	// Use sensor with timeout and poll settings
	if timeoutSeconds == 0 {
		timeoutSeconds = int64(utils.CadenceLongTimeout.Seconds())
	}

	srp := utils.CadenceDefaultSensorRetryPolicy
	srp.ExpirationInterval = time.Duration(timeoutSeconds) * time.Second
	srp.InitialInterval = time.Duration(pollSeconds) * time.Second
	sensorCtx := workflow.WithRetryPolicy(ctx, srp)

	sensorRequest := &v2pb.GetPipelineRunRequest{
		Namespace:  pipelineRun.Namespace,
		Name:       pipelineRun.Name,
		GetOptions: &metav1.GetOptions{},
	}

	if err := workflow.ExecuteActivity(sensorCtx, trigger.Activities.PipelineRunSensor, sensorRequest).Get(sensorCtx, &pipelineRun); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	// Check for FAILED or KILLED states and raise error (matching internal version)
	if pipelineRun.Status.State == v2pb.PIPELINE_RUN_STATE_FAILED ||
		pipelineRun.Status.State == v2pb.PIPELINE_RUN_STATE_KILLED {
		return nil, fmt.Errorf("pipeline run %s failed with status %s", pipelineRun.ObjectMeta.Name, pipelineRun.Status.State.String())
	}

	// Return simplified dict with metadata and status (matching internal version)
	finalResult := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      pipelineRun.ObjectMeta.Name,
			"namespace": pipelineRun.ObjectMeta.Namespace,
		},
		"status": map[string]interface{}{
			"state": pipelineRun.Status.State.String(),
		},
	}

	var resultValue starlark.Value
	if err := utils.AsStar(finalResult, &resultValue); err != nil {
		logger.Error("builtin-error", ext.ZapError(err)...)
		return nil, err
	}

	return resultValue, nil
}
