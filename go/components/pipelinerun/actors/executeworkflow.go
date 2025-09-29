package actors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	pbtypes "github.com/gogo/protobuf/types"
	uberconfig "go.uber.org/config"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	defaultengine "github.com/michelangelo-ai/michelangelo/go/base/conditions/engine"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	"github.com/michelangelo-ai/michelangelo/go/base/config"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
)

const (
	ExecuteWorkflowType        = "Execute Workflow"
	UniflowCadenceWorkflowName = "starlark-workflow" // TODO(#546): fix the typo and make this configurable
	DefaultWorkSpaceRootURL    = "s3://default"      // TODO(#547): make this configurable
	WorkflowEnvironKey         = "environ"
	WorkflowKWArgsKey          = "kwargs"
	WorkflowArgsKey            = "args"
	cacheEnabledVarName        = "CACHE_ENABLED"
	cacheVersionVarName        = "CACHE_VERSION"
	cacheOperationGet          = "GET"
)

// TaskProgress is the struct for the task progress queried from Cadence Workflow
type TaskProgress struct {
	TaskPath       string `json:"task_path"`        // full hierarchical path of the task within the workflow execution tree
	TaskName       string `json:"task_name"`        // name of task
	TaskLog        string `json:"task_log"`         // URL or reference to the task's execution logs
	TaskMessage    string `json:"task_message"`     // contains status messages, error details, or other information from task execution
	TaskState      string `json:"task_state"`       // represents the current execution state (e.g., "running", "succeeded", "failed", "pending")
	StartTime      string `json:"start_time"`       // timestamp when the task execution began
	EndTime        string `json:"end_time"`         // timestamp when the task execution completed
	Output         string `json:"output"`           // contains the serialized output data produced by the task upon completion
	RetryAttemptID string `json:"retry_attempt_id"` // identifies the specific retry attempt for this task execution
}

// ExecuteWorkflowActor handles the execution of workflows for pipeline runs by starting
// and monitoring workflow executions in Cadence/Temporal.
type ExecuteWorkflowActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	logger         *zap.Logger
	workflowClient clientInterfaces.WorkflowClient
	blobStore      *blobstore.BlobStore
	apiHandler     api.Handler
	configProvider uberconfig.Provider
}

// NewExecuteWorkflowActor creates a new ExecuteWorkflowActor with the specified dependencies
// for managing workflow execution.
func NewExecuteWorkflowActor(logger *zap.Logger, workflowClient clientInterfaces.WorkflowClient, blobStore *blobstore.BlobStore, apiHandler api.Handler, configProvider uberconfig.Provider) *ExecuteWorkflowActor {
	return &ExecuteWorkflowActor{
		logger:         logger.With(zap.String("actor", "execute-workflow")),
		workflowClient: workflowClient,
		blobStore:      blobStore,
		apiHandler:     apiHandler,
		configProvider: configProvider,
	}
}

func (a *ExecuteWorkflowActor) Retrieve(ctx context.Context, resource *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)))

	executeWorkflowStep := pipelinerunutils.GetStep(resource, pipelinerunutils.ExecuteWorkflowStepName)
	// Check if workflow step is already in a terminal state.
	if executeWorkflowStep != nil {
		switch executeWorkflowStep.State {
		case v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED:
			logger.Info("workflow execution already completed successfully")
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_TRUE,
			}, nil
		case v2.PIPELINE_RUN_STEP_STATE_FAILED, v2.PIPELINE_RUN_STEP_STATE_KILLED:
			logger.Info("workflow execution failed or was killed")
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, nil
		case v2.PIPELINE_RUN_STEP_STATE_RUNNING:
			if resource.Status.WorkflowRunId != "" && resource.Status.WorkflowId != "" {
				logger.Info("workflow is running")
				return &apipb.Condition{
					Type:   ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_FALSE,
				}, nil
			}
		}
	}

	// Check if workflow needs to be started
	if resource.Status.WorkflowRunId == "" || resource.Status.WorkflowId == "" {
		logger.Info("workflow not started yet, ready to start")
		return &apipb.Condition{
			Type:   ExecuteWorkflowType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Workflow is in progress
	logger.Info("workflow execution in progress")
	return &apipb.Condition{
		Type:   ExecuteWorkflowType,
		Status: apipb.CONDITION_STATUS_FALSE,
	}, nil
}

func (a *ExecuteWorkflowActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	executeWorkflowStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
	if executeWorkflowStep == nil {
		logger.Info("execute workflow step not found, setting to pending")
		executeWorkflowStep = &v2.PipelineRunStepInfo{
			Name:        pipelinerunutils.ExecuteWorkflowStepName,
			DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
			State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
			StartTime:   pbtypes.TimestampNow(),
		}
		pipelineRun.Status.Steps = append(pipelineRun.Status.Steps, executeWorkflowStep)
	}

	newCondition := &apipb.Condition{
		Type:   ExecuteWorkflowType,
		Status: apipb.CONDITION_STATUS_UNKNOWN,
	}

	// If the step is already in a terminal state, just return the condition without any queries
	// This prevents unnecessary status updates which would trigger reconcile loops
	if executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_FAILED || executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_KILLED {
		logger.Info("workflow step already in terminal state, skipping all workflow operations")
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
		if executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_KILLED {
			newCondition.Reason = defaultengine.KillReason
		}
		return newCondition, nil
	}

	if pipelineRun.Spec.Kill {
		workflowTerminated, err := a.processJobTermination(ctx, pipelineRun)
		if err != nil {
			logger.Error("failed to terminate workflow", zap.Error(err))
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to terminate workflow: %w", err)
		}
		// check to see if workflow has been successfully terminated
		if workflowTerminated {
			executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
			executeWorkflowStep.EndTime = pbtypes.TimestampNow()
			newCondition.Status = apipb.CONDITION_STATUS_FALSE
			newCondition.Reason = defaultengine.KillReason
			// Propagate appropriate states to substeps based on their current state
			a.propagateTerminalStateToSubsteps(executeWorkflowStep, v2.PIPELINE_RUN_STEP_STATE_KILLED, defaultengine.KillReason)
			return newCondition, nil
		}
	}

	if pipelineRun.Status.WorkflowRunId == "" || pipelineRun.Status.WorkflowId == "" {
		logger.Info("Workflow run ID is empty, starting workflow")

		// Attempt to retrieve taskList from project.annotations[michelangelo/worker_queue]
		project := &v2.Project{}
		// Try cluster-scoped first (projects might be cluster-scoped resources)
		logger.Info("deciding worker queue...")
		err := a.apiHandler.Get(ctx, pipelineRun.Namespace, pipelineRun.Namespace, &metav1.GetOptions{}, project)
		if err != nil {
			logger.Warn("failed to get project, using config fallback", zap.Error(err), zap.String("projectName", pipelineRun.Namespace))
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to fetch project %w", err)
		}

		taskList, taskListErr := a.getTaskList(project, pipelineRun)
		if taskListErr != nil {
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("get workflow client config: %w", taskListErr)
		}
		if taskList == "" {
			logger.Error("WorkflowClient TaskList is empty")
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("WorkflowClient TaskList is empty")
		}

		workflowExecution, err := a.StartWorkflow(ctx, pipelineRun, taskList)
		if err != nil {
			logger.Error("failed to start workflow",
				zap.Error(err),
				zap.String("operation", "start_workflow"),
				zap.String("namespace", pipelineRun.Namespace),
				zap.String("name", pipelineRun.Name))
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("start workflow for pipeline run %s/%s: %w", pipelineRun.Namespace, pipelineRun.Name, err)
		}
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_RUNNING
		executeWorkflowStep.StartTime = pbtypes.TimestampNow()
		executeWorkflowStep.EndTime = nil
		pipelineRun.Status.WorkflowRunId = workflowExecution.RunID
		pipelineRun.Status.WorkflowId = workflowExecution.ID
		return &apipb.Condition{
			Type:   ExecuteWorkflowType,
			Status: apipb.CONDITION_STATUS_UNKNOWN,
		}, nil
	}

	logger.Info("workflow run ID is not empty, checking workflow status")
	workflowExecution, err := a.workflowClient.GetWorkflowExecutionInfo(ctx, pipelineRun.Status.WorkflowId, pipelineRun.Status.WorkflowRunId)
	if err != nil {
		return nil, fmt.Errorf("get workflow execution info for pipeline run %s/%s (workflow %s, run %s): %w",
			pipelineRun.Namespace, pipelineRun.Name, pipelineRun.Status.WorkflowId, pipelineRun.Status.WorkflowRunId, err)
	}

	// Query and update task-level status for all workflow states
	taskSteps, queryErr := a.constructPipelineRunStepInfo(ctx, pipelineRun)
	if queryErr != nil {
		logger.Error("failed to query task progress", zap.Error(queryErr))
		return nil, queryErr
	} else if len(taskSteps) > 0 {
		executeWorkflowStep.SubSteps = taskSteps
	}

	switch workflowExecution.Status {
	case clientInterfaces.WorkflowExecutionStatusRunning:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_RUNNING
	case clientInterfaces.WorkflowExecutionStatusCompleted:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		newCondition.Status = apipb.CONDITION_STATUS_TRUE
	case clientInterfaces.WorkflowExecutionStatusFailed, clientInterfaces.WorkflowExecutionStatusTimedOut:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
		// Propagate failed state to substeps to ensure no substeps remain in running state
		a.propagateTerminalStateToSubsteps(executeWorkflowStep, v2.PIPELINE_RUN_STEP_STATE_FAILED, "Failed due to workflow failure")
	case clientInterfaces.WorkflowExecutionStatusCanceled, clientInterfaces.WorkflowExecutionStatusTerminated:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
		newCondition.Reason = defaultengine.KillReason
		// Propagate appropriate states to substeps based on their current state
		a.propagateTerminalStateToSubsteps(executeWorkflowStep, v2.PIPELINE_RUN_STEP_STATE_KILLED, defaultengine.KillReason)
	}
	return newCondition, nil
}

func (a *ExecuteWorkflowActor) processJobTermination(ctx context.Context, pipelineRun *v2.PipelineRun) (bool, error) {
	workflowID := pipelineRun.Status.WorkflowId
	runID := pipelineRun.Status.WorkflowRunId

	if workflowID != "" && runID != "" {
		workflowStatus, getWorkflowExecutionInfoError := a.workflowClient.GetWorkflowExecutionInfo(ctx, workflowID, runID)
		if getWorkflowExecutionInfoError == nil {
			if workflowStatus.Status != clientInterfaces.WorkflowExecutionStatusCompleted && workflowStatus.Status != clientInterfaces.WorkflowExecutionStatusTerminated {
				err := a.workflowClient.CancelWorkflow(ctx, workflowID, runID, defaultengine.KillReason)
				// if CancelWorkflow return a non-nil error, the workflow has not been successfully terminated
				if err != nil {
					return false, err
				} else {
					return true, err
				}
			}
		}
	}
	// in this case, the workflow is unable to be terminated because it has not yet been started
	return false, nil
}

func (a *ExecuteWorkflowActor) StartWorkflow(ctx context.Context, pipelineRun *v2.PipelineRun, taskList string) (*clientInterfaces.WorkflowExecution, error) {
	args, kwArgs, envs, err := getWorkflowInputs(pipelineRun)
	if err != nil {
		return nil, fmt.Errorf("get workflow inputs for pipeline run %s/%s: %w", pipelineRun.Namespace, pipelineRun.Name, err)
	}
	err = a.addTaskCacheEnv(ctx, pipelineRun, envs)
	if err != nil {
		return nil, fmt.Errorf("failed to add task cache env: %w", err)
	}
	pipeline := pipelineRun.Status.SourcePipeline.Pipeline
	tarContent, err := a.blobStore.Get(ctx, pipeline.Spec.Manifest.UniflowTar)
	if err != nil {
		return nil, fmt.Errorf("get tar content for pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
	}

	workflowExecution, err := a.workflowClient.StartWorkflow(
		ctx,
		clientInterfaces.StartWorkflowOptions{
			ID:                              pipelineRun.Name,
			TaskList:                        taskList,
			ExecutionStartToCloseTimeout:    7 * 24 * time.Hour,
			DecisionTaskStartToCloseTimeout: 1 * time.Minute,
		},
		UniflowCadenceWorkflowName,
		tarContent,
		"", // .star name has been included in the tarContent
		"", // workflow func name has been included in the tarContent
		args,
		kwArgs,
		envs,
	)
	if err != nil {
		return nil, err
	}

	return workflowExecution, nil
}

func getWorkflowInputs(pipelineRun *v2.PipelineRun) ([]interface{}, []interface{}, map[string]interface{}, error) {
	pipeline := pipelineRun.Status.SourcePipeline.Pipeline
	pipelineConfigMap, err := decodePipelineManifestContent(pipeline.Spec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("decode pipeline manifest content for %s: %w", pipeline.Name, err)
	}

	var args []interface{} = []interface{}{}
	var kwArgs []interface{} = []interface{}{}
	var envs map[string]interface{} = make(map[string]interface{})

	if pipelineConfigMap != nil {
		if _, ok := pipelineConfigMap[WorkflowArgsKey]; ok {
			args = pipelineConfigMap[WorkflowArgsKey].([]interface{})
		}
		if val, ok := pipelineConfigMap[WorkflowKWArgsKey]; ok {
			kwArgs = val.([]interface{})
		}
		if val, ok := pipelineConfigMap[WorkflowEnvironKey]; ok {
			envs = val.(map[string]interface{})
		}
	}

	// Apply DevRun environment overrides if present
	if pipelineRun.Spec.Input != nil {
		if environField := pipelineRun.Spec.Input.Fields["environ"]; environField != nil {
			if err := applyDevRunEnvironmentOverrides(envs, environField.GetStructValue()); err != nil {
				return nil, nil, nil, fmt.Errorf("failed to apply DevRun environment overrides: %w", err)
			}
		}
	}

	envs["MA_NAMESPACE"] = pipelineRun.Namespace
	envs["MA_PIPELINE_RUN_NAME"] = pipelineRun.Name
	if pipelineRun.Spec.WorkspaceRootDir != "" {
		envs["UF_STORAGE_URL"] = pipelineRun.Spec.WorkspaceRootDir
	} else {
		envs["UF_STORAGE_URL"] = DefaultWorkSpaceRootURL
	}
	addTaskImageToEnv(pipelineRun, envs)
	return args, kwArgs, envs, nil
}

func decodePipelineManifestContent(pipelineSpec v2.PipelineSpec) (map[string]interface{}, error) {
	if pipelineSpec.Manifest.Content == nil {
		return map[string]interface{}{}, nil
	}
	pbStruct := &apipb.TypedStruct{}
	err := pbtypes.UnmarshalAny(pipelineSpec.Manifest.Content, pbStruct)
	if err != nil || pbStruct.Value == nil {
		return nil, fmt.Errorf("unmarshal pipeline manifest content to typed struct: %w", err)
	}
	marshaler := &jsonpb.Marshaler{}
	pipelineConfigStr, err := marshaler.MarshalToString(pbStruct.Value)
	if err != nil {
		return nil, fmt.Errorf("marshal pipeline manifest to JSON string: %w", err)
	}
	pipelineConfig := make(map[string]interface{})
	err = json.Unmarshal([]byte(pipelineConfigStr), &pipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("unmarshal pipeline manifest content to map: %w", err)
	}
	return pipelineConfig, nil
}

func (a *ExecuteWorkflowActor) addTaskCacheEnv(ctx context.Context, pipelineRun *v2.PipelineRun, envs map[string]interface{}) error {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	envs[cacheEnabledVarName] = "false"
	envs[cacheVersionVarName] = pipelineRun.Name
	if pipelineRun.Spec.Resume == nil || pipelineRun.Spec.Resume.PipelineRun == nil {
		return nil
	}

	// if resume from a previous run, enable cache
	envs[cacheEnabledVarName] = "true"
	resumePipelineRunID := pipelineRun.Spec.Resume.PipelineRun
	taskCacheVersion := map[string]string{}

	// Loop continues as long as resumePipelineRunID is not nil
	for resumePipelineRunID != nil {
		resumePipelineRun := &v2.PipelineRun{}
		err := pipelinerunutils.GetPipelineRun(ctx, resumePipelineRunID, a.apiHandler, resumePipelineRun)
		if err != nil {
			logger.Error("failed to get resume pipeline run", zap.Error(err))
			return fmt.Errorf("failed to get resume pipeline run: %w", err)
		}
		getTaskCacheVersionFromResumePipelineRun(taskCacheVersion, resumePipelineRun)
		if resumePipelineRun.Spec.Resume == nil || resumePipelineRun.Spec.Resume.PipelineRun == nil {
			break
		}
		logger.Info("Task Cache Version from resume pipeline run", zap.Any("taskCacheVersion", taskCacheVersion), zap.String("resumePipelineRun", resumePipelineRun.Name))
		resumePipelineRunID = resumePipelineRun.Spec.Resume.PipelineRun
	}
	logger.Info("Final Task Cache Version", zap.Any("taskCacheVersion", taskCacheVersion))
	for taskName, cacheVersion := range taskCacheVersion {
		envs[fmt.Sprintf("%s_%s_%s", cacheVersionVarName, cacheOperationGet, taskName)] = cacheVersion
	}
	// Finally, we disable cache for the specified task
	resumeFromTasks := pipelineRun.Spec.Resume.ResumeFrom
	if resumeFromTasks != nil && len(resumeFromTasks) > 0 {
		for _, resumeFromTask := range resumeFromTasks {
			envs[fmt.Sprintf("%s_%s", cacheEnabledVarName, resumeFromTask)] = "false"
		}
	}
	return nil
}

func getTaskCacheVersionFromResumePipelineRun(taskCacheVersion map[string]string, resumePipelineRun *v2.PipelineRun) {
	executeWorkflowStep := getStepInfoByName(pipelinerunutils.ExecuteWorkflowStepName, resumePipelineRun.Status.Steps)
	for _, subStepInfo := range executeWorkflowStep.SubSteps {
		if subStepInfo.StepCachedOutputs != nil && subStepInfo.State == v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED {
			if _, ok := taskCacheVersion[subStepInfo.DisplayName]; !ok {
				taskCacheVersion[subStepInfo.DisplayName] = resumePipelineRun.Name
			}
		}
	}
	return
}

func getStepInfoByName(stepName string, steps []*v2.PipelineRunStepInfo) *v2.PipelineRunStepInfo {
	for _, step := range steps {
		if step.Name == stepName {
			return step
		}
	}
	return nil
}

func addTaskImageToEnv(pipelineRun *v2.PipelineRun, envs map[string]interface{}) {
	imageBuildStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ImageBuildStepName)
	if imageBuildStep.Output != nil {
		for taskName, image := range imageBuildStep.Output.Fields {
			taskImage := image.GetStringValue()
			envName := "UF_TASK_IMAGE"
			if taskName != pipelinerunutils.ImageBuildOutputKey && len(imageBuildStep.Output.Fields) > 1 {
				envName = envName + "_" + taskName
			}
			envs[envName] = taskImage
		}
	}
}

func (a *ExecuteWorkflowActor) Retrieve(ctx context.Context, resource *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)))

	// Check if workflow execution is already completed
	executeWorkflowStep := pipelinerunutils.GetStep(resource, pipelinerunutils.ExecuteWorkflowStepName)
	if executeWorkflowStep != nil {
		switch executeWorkflowStep.State {
		case v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED:
			logger.Info("workflow execution already completed successfully")
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_TRUE,
			}, nil
		case v2.PIPELINE_RUN_STEP_STATE_FAILED, v2.PIPELINE_RUN_STEP_STATE_KILLED:
			logger.Info("workflow execution failed or was killed")
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, nil
		case v2.PIPELINE_RUN_STEP_STATE_RUNNING:
			// Check if workflow is still running by querying workflow client
			if resource.Status.WorkflowRunId != "" && resource.Status.WorkflowId != "" {
				logger.Info("workflow is running, checking status")
				return &apipb.Condition{
					Type:   ExecuteWorkflowType,
					Status: apipb.CONDITION_STATUS_FALSE,
				}, nil
			}
		}
	}

	// Check if workflow needs to be started
	if resource.Status.WorkflowRunId == "" || resource.Status.WorkflowId == "" {
		logger.Info("workflow not started yet")
		return &apipb.Condition{
			Type:   ExecuteWorkflowType,
			Status: apipb.CONDITION_STATUS_FALSE,
		}, nil
	}

	// Workflow is in progress
	logger.Info("workflow execution in progress")
	return &apipb.Condition{
		Type:   ExecuteWorkflowType,
		Status: apipb.CONDITION_STATUS_FALSE,
	}, nil
}

func (a *ExecuteWorkflowActor) GetType() string {
	return ExecuteWorkflowType
}

// propagateTerminalStateToSubsteps updates substep states when the parent workflow reaches a terminal state
// This ensures no substeps remain in RUNNING or PENDING state when the workflow has ended
// - PENDING substeps become INVALID (never started execution)
// - RUNNING substeps become the specified terminal state (FAILED, KILLED, etc.)
// - Terminal states (SUCCEEDED, FAILED, KILLED, SKIPPED) remain unchanged
func (a *ExecuteWorkflowActor) propagateTerminalStateToSubsteps(executeWorkflowStep *v2.PipelineRunStepInfo, terminalState v2.PipelineRunStepState, message string) {
	if executeWorkflowStep.SubSteps == nil {
		return
	}

	for _, substep := range executeWorkflowStep.SubSteps {
		switch substep.State {
		case v2.PIPELINE_RUN_STEP_STATE_PENDING:
			substep.State = v2.PIPELINE_RUN_STEP_STATE_INVALID
			substep.Message = "Workflow ended before step could start"
			// Set end time if not already set
			if substep.EndTime == nil {
				substep.EndTime = pbtypes.TimestampNow()
			}
		case v2.PIPELINE_RUN_STEP_STATE_RUNNING:
			substep.State = terminalState
			substep.Message = message
			// Set end time if not already set
			if substep.EndTime == nil {
				substep.EndTime = pbtypes.TimestampNow()
			}
		default:
			// No change needed for terminal states
		}
	}
}

// applyDevRunEnvironmentOverrides applies DevRun environment variable overrides to the base environment
func applyDevRunEnvironmentOverrides(baseEnv map[string]interface{}, devInput *pbtypes.Struct) error {
	if devInput == nil {
		return nil // No overrides to apply
	}

	// Apply dev input overrides (only accept string values for environment variables)
	for key, value := range devInput.Fields {
		switch value.GetKind().(type) {
		case *pbtypes.Value_StringValue:
			baseEnv[key] = value.GetStringValue()
		default:
			// Environment variables must be strings only
			return fmt.Errorf("environment variable '%s' must be a string, got %T", key, value.GetKind())
		}
	}

	return nil
}

// constructPipelineRunStepInfo queries the workflow for task progress and constructs PipelineRunStepInfo for each task
func (a *ExecuteWorkflowActor) constructPipelineRunStepInfo(ctx context.Context, pipelineRun *v2.PipelineRun) ([]*v2.PipelineRunStepInfo, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	workflowID := pipelineRun.Status.WorkflowId
	runID := pipelineRun.Status.WorkflowRunId

	// Query workflow for task progress
	var workflowProgressStr []string
	err := a.workflowClient.QueryWorkflow(ctx, workflowID, runID, pipelinerunutils.UniflowTaskProgressQueryHandlerKey, &workflowProgressStr)
	if err != nil {
		return []*v2.PipelineRunStepInfo{}, err
	}

	logger.Info("Get Uniflow Progress", zap.Strings("progress", workflowProgressStr))

	// Construct PipelineRunStepInfo for each task
	orderedStepInfo := []*v2.PipelineRunStepInfo{}
	stepMap := make(map[string]*v2.PipelineRunStepInfo)
	stepOrder := []string{}

	for _, progress := range workflowProgressStr {
		var taskProgress TaskProgress
		err := json.Unmarshal([]byte(progress), &taskProgress)
		if err != nil {
			logger.Error("Cannot parse progress string", zap.Error(err), zap.String("progress", progress))
			continue
		}

		taskName := taskProgress.TaskName
		if taskName == "" {
			logger.Error("taskName does not exist", zap.String("progress", progress))
			continue
		}

		if _, existingTask := stepMap[taskName]; !existingTask {
			stepOrder = append(stepOrder, taskName)
			stepMap[taskName] = getStepInfoFromTaskProgress(&taskProgress, pipelineRun.Namespace)
			continue
		}

		// Merge the task progress into the existing step info
		oldStepInfo := stepMap[taskName]
		newStepInfo := getStepInfoFromTaskProgress(&taskProgress, pipelineRun.Namespace)
		stepMap[taskName] = mergePipelineRunStepInfo(oldStepInfo, newStepInfo)
	}

	for _, stepName := range stepOrder {
		orderedStepInfo = append(orderedStepInfo, stepMap[stepName])
	}

	logger.Info("Ordered Step Info", zap.Any("orderedStepInfo", orderedStepInfo))
	return orderedStepInfo, nil
}

func mergePipelineRunStepInfo(oldStepInfo *v2.PipelineRunStepInfo, newStepInfo *v2.PipelineRunStepInfo) *v2.PipelineRunStepInfo {
	mergedStepInfo := proto.Clone(newStepInfo).(*v2.PipelineRunStepInfo)

	// oldStepInfo.AttemptIds is a list of attempt IDs, example: ["0", "1", ...]
	// StepInfo.Resources is a list of driver URLs, example: [<Attempt0-DriverURL>, <Attempt1-DriverURL>, ...]

	// newStepInfo.AttemptIds is a list containing the latest attempt id, example: ["5"]
	// newStepInfo.Resources is a list containing the latest driver URL, example: [<Attempt5-DriverURL>]

	// Our goal is:
	// If the latest attempt ID ALREADY exists in the old step info, update the driver URL
	// If the latest attempt ID DOES NOT exist in the old step info, append the new attempt ID and driver URL

	if attemptIDAlreadyExists(oldStepInfo, newStepInfo) {
		mergedStepInfo.AttemptIds = oldStepInfo.AttemptIds
		mergedStepInfo.Resources = oldStepInfo.Resources
		if len(mergedStepInfo.Resources) > 0 && len(newStepInfo.Resources) > 0 {
			mergedStepInfo.Resources[len(mergedStepInfo.Resources)-1] = newStepInfo.Resources[0]
		}
	} else { // If the new attempt ID does not exist in the old step info, append the new driver URL to the old step info
		mergedStepInfo.Resources = append(oldStepInfo.Resources, newStepInfo.Resources...)
		mergedStepInfo.AttemptIds = append(oldStepInfo.AttemptIds, newStepInfo.AttemptIds...)
	}

	return mergedStepInfo
}

func attemptIDAlreadyExists(oldStepInfo *v2.PipelineRunStepInfo, newStepInfo *v2.PipelineRunStepInfo) bool {
	// oldStepInfo.AttemptIds is a list of attempt IDs, example: ["0", "1", ...]
	// StepInfo.Resources is a list of driver URLs, example: [<Attempt0-DriverURL>, <Attempt1-DriverURL>, ...]

	// newStepInfo.AttemptIds is a list containing the latest attempt id, example: ["5"]
	// newStepInfo.Resources is a list containing the latest driver URL, example: [<Attempt5-DriverURL>]

	// This function checks if the new attempt ID already exists, and is the last item in the old step info

	if len(newStepInfo.AttemptIds) > 0 {
		if len(oldStepInfo.AttemptIds) > 0 && newStepInfo.AttemptIds[0] == oldStepInfo.AttemptIds[len(oldStepInfo.AttemptIds)-1] {
			return true
		}
	}
	return false
}

func getStepInfoFromTaskProgress(taskProgress *TaskProgress, namespace string) *v2.PipelineRunStepInfo {
	stepInfo := &v2.PipelineRunStepInfo{}
	stepInfo.Name = taskProgress.TaskPath
	stepInfo.DisplayName = taskProgress.TaskName
	stepInfo.LogUrl = taskProgress.TaskLog

	if taskProgress.StartTime != "" {
		// parse utc time str 2024-06-10 17:53:20 to time.Time
		startTime, err := time.Parse("2006-01-02 15:04:05", taskProgress.StartTime)
		if err == nil {
			stepInfo.StartTime = &pbtypes.Timestamp{Seconds: startTime.Unix()}
		}
	}

	if taskProgress.EndTime != "" {
		// parse utc time str 2024-06-10 17:53:20 to time.Time
		endTime, err := time.Parse("2006-01-02 15:04:05", taskProgress.EndTime)
		if err == nil {
			stepInfo.EndTime = &pbtypes.Timestamp{Seconds: endTime.Unix()}
		}
	}

	if taskProgress.Output != "" {
		stepInfo.StepCachedOutputs = &v2.PipelineRunStepCachedOutputs{
			IntermediateVars: []*apipb.ResourceIdentifier{
				{
					Namespace: namespace,
					Name:      taskProgress.Output,
				},
			},
		}
	}

	switch taskProgress.TaskState {
	case pipelinerunutils.UniflowTaskStateRunning:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_RUNNING
	case pipelinerunutils.UniflowTaskStateSucceeded:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED
	case pipelinerunutils.UniflowTaskStateFailed:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		stepInfo.Message = taskProgress.TaskMessage
	case pipelinerunutils.UniflowTaskStateKilled:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
		stepInfo.Message = taskProgress.TaskMessage
	case pipelinerunutils.UniflowTaskStateSkipped:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_SKIPPED
	default:
		stepInfo.State = v2.PIPELINE_RUN_STEP_STATE_PENDING
	}

	if taskProgress.RetryAttemptID != "" {
		stepInfo.Resources = []*v2.PipelineRunResource{
			{
				Resource: &v2.PipelineRunResource_ExternalResource{
					ExternalResource: &v2.ExternalResource{
						Name: fmt.Sprintf("Attempt%s-DriverURL", taskProgress.RetryAttemptID),
						Url:  taskProgress.TaskLog,
					},
				},
			},
		}
		stepInfo.AttemptIds = []string{taskProgress.RetryAttemptID}
	}

	return stepInfo
}

func (a *ExecuteWorkflowActor) getTaskList(project *v2.Project, pipelineRun *v2.PipelineRun) (string, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	var taskList string
	if project.GetMetadata().GetAnnotations() != nil {
		if workerQueue, exists := project.GetMetadata().GetAnnotations()["michelangelo/worker_queue"]; exists && workerQueue != "" {
			taskList = workerQueue
			logger.Info("using worker queue from project annotations", zap.String("taskList", taskList))
		}
	} else {
		logger.Info("project annotations", zap.String("annotation", project.GetMetadata().GetAnnotations()["michelangelo/worker_queue"]))
	}
	logger.Info("task list", zap.String("taskList", taskList))

	// If project CR does not have worker_queue specified, as a fallback, retrieve taskList from config
	if taskList == "" {
		workflowConfig, getWorkflowClientConfigErr := config.GetWorkflowClientConfig(a.configProvider)
		if getWorkflowClientConfigErr != nil {
			logger.Error("failed to get workflow client config", zap.Error(getWorkflowClientConfigErr))
			return "", getWorkflowClientConfigErr
		}
		taskList = workflowConfig.TaskList
	}
	return taskList, nil
}
