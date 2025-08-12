package actors

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
)

const (
	ExecuteWorkflowType        = "Execute Workflow"
	DefaultWorkflowTaskList    = "default"
	UniflowCadenceWorkflowName = "starlark-worklow" // TODO: fix the typo and make this configurable
	DefaultWorkSpaceRootURL    = "s3://default"     // TODO: make this configurable
	WorkflowEnvironKey         = "environ"
	WorkflowKWArgsKey          = "kwargs"
	WorkflowArgsKey            = "args"
)

// TaskProgress is the struct for the task progress queried from Cadence Workflow
type TaskProgress struct {
	TaskPath       string `json:"task_path"`
	TaskName       string `json:"task_name"`
	TaskLog        string `json:"task_log"`
	TaskMessage    string `json:"task_message"`
	TaskState      string `json:"task_state"`
	StartTime      string `json:"start_time"`
	EndTime        string `json:"end_time"`
	Output         string `json:"output"`
	RetryAttemptID string `json:"retry_attempt_id"`
}

type ExecuteWorkflowActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	logger         *zap.Logger
	workflowClient clientInterfaces.WorkflowClient
	blobStore      *blobstore.BlobStore
}

func NewExecuteWorkflowActor(logger *zap.Logger, workflowClient clientInterfaces.WorkflowClient, blobStore *blobstore.BlobStore) *ExecuteWorkflowActor {
	return &ExecuteWorkflowActor{
		logger:         logger.With(zap.String("actor", "execute-workflow")),
		workflowClient: workflowClient,
		blobStore:      blobStore,
	}
}

func (a *ExecuteWorkflowActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	if previousCondition == nil {
		logger.Info("pipeline run has no previous condition, setting to unknown, adding step")
		pipelineRun.Status.Steps = append(pipelineRun.Status.Steps, &v2.PipelineRunStepInfo{
			Name:        pipelinerunutils.ExecuteWorkflowStepName,
			DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
			State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
			StartTime:   pbtypes.TimestampNow(),
		})
		return &apipb.Condition{
			Type:   ExecuteWorkflowType,
			Status: apipb.CONDITION_STATUS_UNKNOWN,
		}, nil
	}

	executeWorkflowStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)

	if pipelineRun.Status.WorkflowRunId == "" || pipelineRun.Status.WorkflowId == "" {
		logger.Info("Workflow run ID is empty, starting workflow")
		workflowExecution, err := a.StartWorkflow(ctx, pipelineRun)
		if err != nil {
			logger.Error("failed to start workflow", zap.Error(err))
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, fmt.Errorf("failed to start workflow: %v", err)
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
	logger.Info("Workflow run ID is not empty, checking workflow status")
	workflowExecution, err := a.workflowClient.GetWorkflowExecutionInfo(ctx, pipelineRun.Status.WorkflowId, pipelineRun.Status.WorkflowRunId)
	if err != nil {
		return nil, err
	}
	newCondition := &apipb.Condition{
		Type:   ExecuteWorkflowType,
		Status: apipb.CONDITION_STATUS_UNKNOWN,
	}
	switch workflowExecution.Status {
	case clientInterfaces.WorkflowExecutionStatusRunning:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_RUNNING
		// Query and update task-level status
		taskSteps, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			logger.Warn("Failed to query task progress", zap.Error(err))
		} else if len(taskSteps) > 0 {
			executeWorkflowStep.SubSteps = taskSteps
		}
	case clientInterfaces.WorkflowExecutionStatusCompleted:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		// Query final task-level status
		taskSteps, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			logger.Warn("Failed to query final task progress", zap.Error(err))
		} else if len(taskSteps) > 0 {
			executeWorkflowStep.SubSteps = taskSteps
		}
		newCondition.Status = apipb.CONDITION_STATUS_TRUE
	case clientInterfaces.WorkflowExecutionStatusFailed, clientInterfaces.WorkflowExecutionStatusTimedOut:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		// Query task-level status to identify which task failed
		taskSteps, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			logger.Warn("Failed to query task progress on failure", zap.Error(err))
		} else if len(taskSteps) > 0 {
			executeWorkflowStep.SubSteps = taskSteps
		}
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
	case clientInterfaces.WorkflowExecutionStatusCanceled, clientInterfaces.WorkflowExecutionStatusTerminated:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		// Query task-level status
		taskSteps, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			logger.Warn("Failed to query task progress on cancel", zap.Error(err))
		} else if len(taskSteps) > 0 {
			executeWorkflowStep.SubSteps = taskSteps
		}
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
	}
	return newCondition, nil
}

func (a *ExecuteWorkflowActor) StartWorkflow(ctx context.Context, pipelineRun *v2.PipelineRun) (*clientInterfaces.WorkflowExecution, error) {

	args, kwArgs, envs, err := getWorkflowInputs(pipelineRun)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow inputs: %v", err)
	}
	pipeline := pipelineRun.Status.SourcePipeline.Pipeline
	tarContent, err := a.blobStore.Get(ctx, pipeline.Spec.Manifest.UniflowTar)
	if err != nil {
		return nil, fmt.Errorf("failed to get tar content: %v", err)
	}

	workflowExecution, err := a.workflowClient.StartWorkflow(
		ctx,
		clientInterfaces.StartWorkflowOptions{
			ID:                              pipelineRun.Name,
			TaskList:                        DefaultWorkflowTaskList, // TODO: make this configurable
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
		return nil, nil, nil, fmt.Errorf("failed to decode pipeline manifest content: %v", err)
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

	envs["MA_NAMESPACE"] = pipelineRun.Namespace
	envs["MA_PIPELINE_RUN_NAME"] = pipelineRun.Name
	envs["UF_STORAGE_URL"] = DefaultWorkSpaceRootURL
	addTaskImageToEnv(pipelineRun, envs)
	return args, kwArgs, envs, nil
}

func decodePipelineManifestContent(pipelineSpec v2.PipelineSpec) (map[string]interface{}, error) {
	if pipelineSpec.Manifest.Content == nil {
		return map[string]interface{}{}, nil
	}
	pbStruct := &apipb.TypedStruct{}
	fmt.Println(reflect.TypeOf(pbStruct))
	fmt.Println(proto.MessageName(pbStruct))
	t := proto.MessageType("michelangelo.api.TypedStruct")
	fmt.Println(t)
	err := pbtypes.UnmarshalAny(pipelineSpec.Manifest.Content, pbStruct)
	if err != nil || pbStruct.Value == nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline manifest content to typed struct: %v", err)
	}
	marshaler := &jsonpb.Marshaler{}
	pipelineConfigStr, _ := marshaler.MarshalToString(pbStruct.Value)
	pipelineConfig := make(map[string]interface{})
	err = json.Unmarshal([]byte(pipelineConfigStr), &pipelineConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pipeline manifest content to map : %v", err)
	}
	return pipelineConfig, nil
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

func (a *ExecuteWorkflowActor) GetType() string {
	return ExecuteWorkflowType
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
		return nil, fmt.Errorf("failed to query workflow task progress: %w", err)
	}
	
	// Construct PipelineRunStepInfo for each task
	orderedStepInfo := []*v2.PipelineRunStepInfo{}
	stepMap := make(map[string]*v2.PipelineRunStepInfo)
	stepOrder := []string{}
	
	for _, progress := range workflowProgressStr {
		var taskProgress TaskProgress
		err := json.Unmarshal([]byte(progress), &taskProgress)
		if err != nil {
			logger.Error("Failed to parse task progress", zap.Error(err), zap.String("progress", progress))
			continue
		}
		
		taskName := taskProgress.TaskName
		if taskName == "" {
			logger.Warn("Task name is empty", zap.String("progress", progress))
			continue
		}
		
		if _, exists := stepMap[taskName]; !exists {
			stepOrder = append(stepOrder, taskName)
			stepMap[taskName] = a.getStepInfoFromTaskProgress(&taskProgress, pipelineRun.Namespace)
		} else {
			// Merge task progress for retries
			oldStepInfo := stepMap[taskName]
			newStepInfo := a.getStepInfoFromTaskProgress(&taskProgress, pipelineRun.Namespace)
			stepMap[taskName] = a.mergePipelineRunStepInfo(oldStepInfo, newStepInfo)
		}
	}
	
	for _, stepName := range stepOrder {
		orderedStepInfo = append(orderedStepInfo, stepMap[stepName])
	}
	
	logger.Debug("Constructed task step info", zap.Int("count", len(orderedStepInfo)))
	return orderedStepInfo, nil
}

// getStepInfoFromTaskProgress converts TaskProgress to PipelineRunStepInfo
func (a *ExecuteWorkflowActor) getStepInfoFromTaskProgress(taskProgress *TaskProgress, namespace string) *v2.PipelineRunStepInfo {
	stepInfo := &v2.PipelineRunStepInfo{
		Name:        taskProgress.TaskPath,
		DisplayName: taskProgress.TaskName,
	}
	
	// Parse start time
	if taskProgress.StartTime != "" {
		startTime, err := time.Parse("2006-01-02 15:04:05", taskProgress.StartTime)
		if err == nil {
			stepInfo.StartTime = &pbtypes.Timestamp{Seconds: startTime.Unix()}
		}
	}
	
	// Parse end time
	if taskProgress.EndTime != "" {
		endTime, err := time.Parse("2006-01-02 15:04:05", taskProgress.EndTime)
		if err == nil {
			stepInfo.EndTime = &pbtypes.Timestamp{Seconds: endTime.Unix()}
		}
	}
	
	// Set output if available
	if taskProgress.Output != "" {
		stepInfo.Output = &pbtypes.Struct{
			Fields: map[string]*pbtypes.Value{
				"output": {
					Kind: &pbtypes.Value_StringValue{
						StringValue: taskProgress.Output,
					},
				},
			},
		}
	}
	
	// Map task state to PipelineRunStepState
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
	
	return stepInfo
}

// mergePipelineRunStepInfo merges retry attempt information into existing step info
func (a *ExecuteWorkflowActor) mergePipelineRunStepInfo(oldStepInfo, newStepInfo *v2.PipelineRunStepInfo) *v2.PipelineRunStepInfo {
	// Use the latest step info as base
	mergedStepInfo := proto.Clone(newStepInfo).(*v2.PipelineRunStepInfo)
	
	// Preserve sub-steps from old step if they exist and new doesn't have them
	if len(oldStepInfo.SubSteps) > 0 && len(newStepInfo.SubSteps) == 0 {
		mergedStepInfo.SubSteps = oldStepInfo.SubSteps
	}
	
	return mergedStepInfo
}
