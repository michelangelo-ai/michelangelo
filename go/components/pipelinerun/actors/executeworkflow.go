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
		orderedStepInfo, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			return nil, err
		}
		executeWorkflowStep.SubSteps = orderedStepInfo
	case clientInterfaces.WorkflowExecutionStatusCompleted:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		orderedStepInfo, err := a.constructPipelineRunStepInfo(ctx, pipelineRun)
		if err != nil {
			return nil, err
		}
		executeWorkflowStep.SubSteps = orderedStepInfo
		newCondition.Status = apipb.CONDITION_STATUS_TRUE
	case clientInterfaces.WorkflowExecutionStatusFailed, clientInterfaces.WorkflowExecutionStatusTimedOut:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_FAILED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
	case clientInterfaces.WorkflowExecutionStatusCanceled, clientInterfaces.WorkflowExecutionStatusTerminated:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
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

func updateUniflowStatus(ctx context.Context, pipelineRun *v2.PipelineRun) error {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))

	// update pipelinerun step info
	err := updatePipelineRunStepInfo(ctx, pipelineRun)
	if err != nil {
		logger.Error(err, "Error updating PipelineRun Step Info")
		return err
	}

	// update final status
	err = updateFinalStatus(ctx, pipelineRun)
	if err != nil {
		logger.Error(err, "Error update final status")
		// do not return error here, we still want to update the substep status
		// the GetWorkflowExecutionInfo might fail due to transient issue
		// one example is when MA queries a passive cadecne domain, the workflow status is not fully replicated yet from active region to the passive region
	}
	return nil
}

func (a *ExecuteWorkflowActor) updateFinalStatus(ctx context.Context, pipelineRun *v2.PipelineRun) error {
	workflowID := pipelineRun.Status.OrchestrationWorkflowId
	runID := pipelineRun.Status.OrchestrationExecutionId
	cadenceDomain := a.config.CadenceDomain
	cadenceService := a.config.CadenceService

	// check the workflow final status
	workflowExecutionInfo, err := a.workflowClient.GetWorkflowExecutionInfo(ctx, workflowID, runID)
	if err != nil {
		return err
	}
	if workflowExecutionInfo == nil {
		return fmt.Errorf("workflow execution info is nil")
	}

	if workflowExecutionInfo.IsSetCloseStatus() {
		closeStatus := workflowExecutionInfo.GetCloseStatus()
		executeWorkflow := getStepInfoByName(pipelinerunutils.ExecuteWorkflowStepName, pipelineRun.Status.Steps)
		executeWorkflow.EndTime = pbtypes.TimestampNow()
		switch closeStatus {
		case cadenceShared.WorkflowExecutionCloseStatusCompleted:
			executeWorkflow.State = v2beta1.PIPELINE_RUN_STEP_STATE_SUCCEEDED
		case cadenceShared.WorkflowExecutionCloseStatusCanceled:
			executeWorkflow.State = v2beta1.PIPELINE_RUN_STEP_STATE_KILLED
			executeWorkflow.Message = "Cadence Workflow Execution Close Status: " + closeStatus.String()
		case cadenceShared.WorkflowExecutionCloseStatusTerminated, cadenceShared.WorkflowExecutionCloseStatusFailed, cadenceShared.WorkflowExecutionCloseStatusTimedOut, cadenceShared.WorkflowExecutionCloseStatusContinuedAsNew: // We will not retry at pipelinerun controller
			executeWorkflow.State = v2beta1.PIPELINE_RUN_STEP_STATE_FAILED
			executeWorkflow.Message = "Cadence Workflow Execution Close Status: " + closeStatus.String()
		}
	}
	return nil

}

func updatePipelineRunStepInfo(ctx context.Context, pipelineRun *v2.PipelineRun) error {
	newStepInfoList, err := constructPipelineRunStepInfo(ctx, pipelineRun)
	if err != nil {
		return err
	}
	executeWorkflow := getStepInfoByName(pipelinerunutils.ExecuteWorkflowStepName, pipelineRun.Status.Steps)

	if len(newStepInfoList) > 0 {
		executeWorkflow.SubSteps = newStepInfoList
	}
	return nil
}

func getStepInfoByName(stepName string, steps []*v2.PipelineRunStepInfo) *v2.PipelineRunStepInfo {
	for _, step := range steps {
		if step.Name == stepName {
			return step
		}
	}
	return nil
}

func (a *ExecuteWorkflowActor) constructPipelineRunStepInfo(ctx context.Context, pipelineRun *v2.PipelineRun) ([]*v2.PipelineRunStepInfo, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	workflowID := pipelineRun.Status.WorkflowId
	runID := pipelineRun.Status.WorkflowRunId
	cadenceDomain := a.config.CadenceDomain
	cadenceService := a.config.CadenceService
	// check the workflow progress
	workflowProgressStr := []string{}
	err := a.workflowClient.QueryWorkflow(ctx, workflowID, runID, pipelinerunutils.UniflowTaskProgressQueryHandlerKey, &workflowProgressStr)
	if err != nil {
		return []*v2.PipelineRunStepInfo{}, err
	}
	// construct the pipelineRunStepInfo
	orderedStepInfo := []*v2.PipelineRunStepInfo{}
	stepMap := make(map[string]*v2.PipelineRunStepInfo)
	stepOrder := []string{}
	for _, progress := range workflowProgressStr {
		var taskProgress TaskProgress
		err := json.Unmarshal([]byte(progress), &taskProgress)
		if err != nil {
			logger.Error(fmt.Errorf("Can not parase progress string"), err.Error(), "progress", progress)
			continue
		}
		taskName := taskProgress.TaskName
		if taskName == "" {
			logger.Error(fmt.Errorf("taskName does not exist"), "taskName does not exist", "progress", progress)
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
	logger.Info("Ordered Step Info", "orderedStepInfo", orderedStepInfo)
	return orderedStepInfo, nil
}

func mergePipelineRunStepInfo(oldStepInfo *v2.PipelineRunStepInfo, newStepInfo *v2.PipelineRunStepInfo) *v2.PipelineRunStepInfo {

	mergedStepInfo := proto.Clone(newStepInfo).(*v2.PipelineRunStepInfo)

	// oldStepInfo.AttemptIds is a list of attempt IDs, example: ["0", "1", ...]
	// StepInfo.Resources is a list of driver URLs, example: [<Attempt0-DriverURL>, <Attempt1-DriverURL>, ...]

	// newStepInfo.AttemptIds is a list containiing the latest attempt id, example: ["5"]
	// newStepInfo.Resources is a list containing the latest driver URL, example: [<Attempt5-DriverURL>]

	// Our goal is:
	// If the latest attempt ID ALREADY exists in the old step info, update the driver URL
	// If the latest attempt ID DOES NOT exist in the old step info, append the new attempt ID and driver URL

	if attemptIDAlreadyExists(oldStepInfo, newStepInfo) {

		mergedStepInfo.AttemptIds = oldStepInfo.AttemptIds

		mergedStepInfo.Resources = oldStepInfo.Resources
		mergedStepInfo.Resources[len(mergedStepInfo.Resources)-1] = newStepInfo.Resources[0]

	} else { // If the new attempt ID does not exist in the old step info, append the new driver URL to the old step info
		mergedStepInfo.Resources = append(oldStepInfo.Resources, newStepInfo.Resources...)
		mergedStepInfo.AttemptIds = append(oldStepInfo.AttemptIds, newStepInfo.AttemptIds...)
	}

	return mergedStepInfo
}

func attemptIDAlreadyExists(oldStepInfo *v2beta1.PipelineRunStepInfo, newStepInfo *v2beta1.PipelineRunStepInfo) bool {

	// oldStepInfo.AttemptIds is a list of attempt IDs, example: ["0", "1", ...]
	// StepInfo.Resources is a list of driver URLs, example: [<Attempt0-DriverURL>, <Attempt1-DriverURL>, ...]

	// newStepInfo.AttemptIds is a list containiing the latest attempt id, example: ["5"]
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
			&v2.PipelineRunResource{
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