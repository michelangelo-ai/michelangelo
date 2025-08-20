package actors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	pbtypes "github.com/gogo/protobuf/types"
	"github.com/michelangelo-ai/michelangelo/go/api"
	"github.com/michelangelo-ai/michelangelo/go/base/blobstore"
	conditionInterfaces "github.com/michelangelo-ai/michelangelo/go/base/conditions/interfaces"
	clientInterfaces "github.com/michelangelo-ai/michelangelo/go/base/workflowclient/interface"
	pipelinerunutils "github.com/michelangelo-ai/michelangelo/go/components/pipelinerun/actors/utils"
	apipb "github.com/michelangelo-ai/michelangelo/proto/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	CacheEnabledVarName        = "CACHE_ENABLED"
	CacheVersionVarName        = "CACHE_VERSION"
	CacheOperationGet          = "GET"
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
	apiHandler     api.Handler
}

func NewExecuteWorkflowActor(logger *zap.Logger, workflowClient clientInterfaces.WorkflowClient, blobStore *blobstore.BlobStore, apiHandler api.Handler) *ExecuteWorkflowActor {
	return &ExecuteWorkflowActor{
		logger:         logger.With(zap.String("actor", "execute-workflow")),
		workflowClient: workflowClient,
		blobStore:      blobStore,
		apiHandler:     apiHandler,
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

	// Query and update task-level status for all workflow states
	taskSteps, queryErr := a.constructPipelineRunStepInfo(ctx, pipelineRun)
	if queryErr != nil {
		logger.Warn("Failed to query task progress", zap.Error(queryErr))
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
	case clientInterfaces.WorkflowExecutionStatusCanceled, clientInterfaces.WorkflowExecutionStatusTerminated:
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_KILLED
		executeWorkflowStep.EndTime = pbtypes.TimestampNow()
		newCondition.Status = apipb.CONDITION_STATUS_FALSE
	}
	return newCondition, nil
}

func (a *ExecuteWorkflowActor) StartWorkflow(ctx context.Context, pipelineRun *v2.PipelineRun) (*clientInterfaces.WorkflowExecution, error) {

	args, kwArgs, envs, err := a.getWorkflowInputs(ctx, pipelineRun)
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

func (a *ExecuteWorkflowActor) getWorkflowInputs(ctx context.Context, pipelineRun *v2.PipelineRun) ([]interface{}, []interface{}, map[string]interface{}, error) {

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

	// Add task cache environment variables following the same logic as internal Uber implementation
	err = a.addTaskCacheEnv(ctx, pipelineRun, envs)
	if err != nil {
		a.logger.Error("failed to add task cache env", zap.Error(err))
		return nil, nil, nil, fmt.Errorf("failed to add task cache env: %v", err)
	}

	return args, kwArgs, envs, nil
}

func decodePipelineManifestContent(pipelineSpec v2.PipelineSpec) (map[string]interface{}, error) {
	if pipelineSpec.Manifest.Content == nil {
		return map[string]interface{}{}, nil
	}
	pbStruct := &apipb.TypedStruct{}
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
	if imageBuildStep != nil && imageBuildStep.Output != nil {
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

// addTaskCacheEnv adds task cache environment variables following the same logic as internal Uber implementation
func (a *ExecuteWorkflowActor) addTaskCacheEnv(ctx context.Context, pipelineRun *v2.PipelineRun, envs map[string]interface{}) error {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))
	envs[CacheEnabledVarName] = "false"
	envs[CacheVersionVarName] = pipelineRun.Name
	
	if pipelineRun.Spec.Resume == nil || pipelineRun.Spec.Resume.PipelineRun == nil {
		return nil
	}

	// if resume from a previous run, enable cache
	envs[CacheEnabledVarName] = "true"
	resumePipelineRunID := pipelineRun.Spec.Resume.PipelineRun
	taskCacheVersion := map[string]string{}

	// Loop continues as long as resumePipelineRunID is not nil
	for resumePipelineRunID != nil {
		resumePipelineRun := &v2.PipelineRun{}
		err := a.apiHandler.Get(ctx, resumePipelineRunID.Namespace, resumePipelineRunID.Name, &metav1.GetOptions{}, resumePipelineRun)
		if err != nil {
			logger.Error("failed to get resume pipeline run", zap.Error(err))
			return fmt.Errorf("failed to get resume pipeline run: %v", err)
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
		envs[fmt.Sprintf("%s_%s_%s", CacheVersionVarName, CacheOperationGet, taskName)] = cacheVersion
	}
	// Finally, we disable cache for the specified task
	resumeFromTasks := pipelineRun.Spec.Resume.ResumeFrom
	if resumeFromTasks != nil && len(resumeFromTasks) > 0 {
		for _, resumeFromTask := range resumeFromTasks {
			envs[fmt.Sprintf("%s_%s", CacheEnabledVarName, resumeFromTask)] = "false"
		}
	}
	return nil
}

// getTaskCacheVersionFromResumePipelineRun extracts task cache version information from a resume pipeline run
func getTaskCacheVersionFromResumePipelineRun(taskCacheVersion map[string]string, resumePipelineRun *v2.PipelineRun) {
	executeWorkflowStep := pipelinerunutils.GetStep(resumePipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
	if executeWorkflowStep == nil {
		return
	}
	for _, subStepInfo := range executeWorkflowStep.SubSteps {
		if subStepInfo.StepCachedOutputs != nil && subStepInfo.State == v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED {
			if _, ok := taskCacheVersion[subStepInfo.DisplayName]; !ok {
				taskCacheVersion[subStepInfo.DisplayName] = resumePipelineRun.Name
			}
		}
	}
}
