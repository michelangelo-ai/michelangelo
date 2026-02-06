package actors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
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
	apipb "github.com/michelangelo-ai/michelangelo/proto-go/api"
	v2 "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	// ExecuteWorkflowType is the condition type for the workflow execution stage.
	ExecuteWorkflowType = "Execute Workflow"

	// UniflowCadenceWorkflowName is the workflow type name registered in Cadence/Temporal.
	UniflowCadenceWorkflowName = "starlark-workflow" // TODO(#546): fix the typo and make this configurable

	// DefaultWorkSpaceRootURL is the default S3 location for workflow artifacts.
	DefaultWorkSpaceRootURL = "s3://default" // TODO(#547): make this configurable

	// Workflow input parameter keys for uniflow pipeline
	WorkflowEnvironKey = "environ"
	WorkflowKWArgsKey  = "kw_args"
	WorkflowArgsKey    = "args"

	// Workflow input parameter keys for canvas flex pipeline
	WorkflowTaskConfigsKey = "task_configs"
	WorkflowConfigKey      = "workflow_config"

	// Cache-related environment variable names.
	cacheEnabledVarName = "CACHE_ENABLED"
	cacheVersionVarName = "CACHE_VERSION"
	cacheOperationGet   = "GET"
)

// TaskProgress represents the execution state of a workflow task.
//
// This struct is populated from Cadence/Temporal workflow queries and provides
// detailed information about individual task execution within a pipeline run.
// Enhanced with activity IDs for precise retry control.
type TaskProgress struct {
	TaskPath          string   `json:"task_path"`           // full hierarchical path of the task within the workflow execution tree
	TaskName          string   `json:"task_name"`           // name of task
	TaskLog           string   `json:"task_log"`            // URL or reference to the task's execution logs
	TaskMessage       string   `json:"task_message"`        // contains status messages, error details, or other information from task execution
	TaskState         string   `json:"task_state"`          // represents the current execution state (e.g., "running", "succeeded", "failed", "pending")
	StartTime         string   `json:"start_time"`          // timestamp when the task execution began
	EndTime           string   `json:"end_time"`            // timestamp when the task execution completed
	Output            string   `json:"output"`              // contains the serialized output data produced by the task upon completion
	RetryAttemptID    string   `json:"retry_attempt_id"`    // identifies the specific retry attempt for this task execution
	FirstActivityID   string   `json:"first_activity_id"`   // ID of the first activity in this task
	CurrentActivityID string   `json:"current_activity_id"` // ID of the currently executing activity
	ActivitySequence  []string `json:"activity_sequence"`   // Ordered list of activity IDs in this task
}

// RetryConfig removed - was only used for automatic retry configuration which is now handled at Starlark level

// TaskRetryMetadata removed - was only used for automatic retry logic which is now handled at Starlark level

// ManualRetryTaskRequest is deprecated in favor of the new workflow_run_id based retry approach.
// Retry is now triggered via RetryInfo field in PipelineRunSpec with workflow_run_id trigger condition.

// ExecuteWorkflowActor implements the workflow execution stage of pipeline runs.
//
// This actor is responsible for:
//   - Starting Cadence/Temporal workflows with pipeline configuration
//   - Monitoring workflow execution status
//   - Querying and updating task-level progress
//   - Handling workflow termination and cancellation
//   - Managing task caching for pipeline run resumption
//   - Implementing task-level retry functionality using workflow reset
//
// The actor integrates with the workflow client to execute Starlark-based ML workflows
// and tracks detailed execution progress including individual task states, logs, and outputs.
//
// ## Task-Level Retry Implementation
//
// The retry functionality provides both automatic and manual retry capabilities for failed tasks:
//
// ### Automatic Retry Features:
// - Detects retriable failures based on configurable failure patterns
// - Enforces per-task retry limits and intervals
// - Uses workflow reset to restart from appropriate decision points
// - Maintains full audit trail of retry attempts and failures
// - Prevents race conditions and concurrent retry attempts
//
// ### Manual Retry Features:
// - Triggered via RetryInfo field in PipelineRunSpec with workflow_run_id trigger condition
// - Uses workflow reset for precise retry control without duplicate processing
// - Automatically queries activity IDs for reset boundary calculation
// - Updates pipeline run status and clears retry info after successful processing
//
// ### Configuration:
// - Configurable via pipeline run annotations (michelangelo.ai/retry-*)
// - Per-task overrides supported via pipeline spec input
// - Default patterns include OOM, system errors, infrastructure failures
//
// ### Reset Strategy:
// - Finds appropriate workflow/decision task completed events as reset points
// - Resets workflow execution to just before the failed task
// - Updates pipeline run status with new workflow run ID
// - Preserves task progress history and attempt metadata
//
// ### Integration with Starlark Workflows:
// - Compatible with report_progress() function retry_attempt_id field
// - Handles retry logic from both workflow engine and controller side
// - Supports existing task caching and output management
type ExecuteWorkflowActor struct {
	conditionInterfaces.ConditionActor[*v2.PipelineRun]
	logger         *zap.Logger
	workflowClient clientInterfaces.WorkflowClient
	blobStore      *blobstore.BlobStore
	apiHandler     api.Handler
	configProvider uberconfig.Provider
	// retryMetadata and retryMutex removed - were only used for automatic retry tracking
}

// NewExecuteWorkflowActor creates a new ExecuteWorkflowActor with the specified dependencies
// for managing workflow execution.
func NewExecuteWorkflowActor(logger *zap.Logger, workflowClient clientInterfaces.WorkflowClient, blobStore *blobstore.BlobStore, apiHandler api.Handler, configProvider uberconfig.Provider) *ExecuteWorkflowActor {
	actorLogger := logger.With(zap.String("actor", "execute-workflow"))
	return &ExecuteWorkflowActor{
		logger:         actorLogger,
		workflowClient: workflowClient,
		blobStore:      blobStore,
		apiHandler:     apiHandler,
		configProvider: configProvider,
		// retryMetadata initialization removed
	}
}

// Retrieve checks the current state of workflow execution.
//
// It examines the workflow execution step to determine if the workflow has completed,
// is running, or needs to be started. The method returns TRUE if execution is complete,
// FALSE if it's in progress or needs to start.
//
// Returns an appropriate condition based on the workflow execution state.
func (a *ExecuteWorkflowActor) Retrieve(ctx context.Context, resource *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name)))

	// Check for retry scenario first - if RetryInfo is present and workflow run IDs differ, allow retry processing
	retryInfo := resource.Spec.RetryInfo
	if retryInfo != nil && retryInfo.ActivityId != "" {
		// Check the trigger condition: only process if workflowRunId differs from current status
		if retryInfo.WorkflowRunId != "" && retryInfo.WorkflowRunId == resource.Status.WorkflowRunId {
			logger.Info("retry scenario detected - allowing retry processing",
				zap.String("retryWorkflowRunId", retryInfo.WorkflowRunId),
				zap.String("currentWorkflowRunId", resource.Status.WorkflowRunId),
				zap.String("activityId", retryInfo.ActivityId))
			return &apipb.Condition{
				Type:   ExecuteWorkflowType,
				Status: apipb.CONDITION_STATUS_FALSE,
			}, nil
		}
	}

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

// GetWorkflowUrl constructs the monitoring URL for workflow execution logs and status.
//
// This method builds a formatted URL that points to the workflow engine's web UI,
// allowing users to monitor workflow execution progress, view logs, and inspect
// task-level details. The URL format is configured via the workflow client settings
// and typically points to the Cadence or Temporal web interface.
//
// Parameters:
//   - name: The workflow execution name (usually the pipeline run name)
//
// Returns a formatted URL string for the workflow monitoring interface, or an
// empty string if the workflow client configuration cannot be retrieved.
func (a *ExecuteWorkflowActor) GetWorkflowUrl(name string) string {
	workflowConfig, getWorkflowClientConfigErr := config.GetWorkflowClientConfig(a.configProvider)
	if getWorkflowClientConfigErr != nil {
		return ""
	}

	// Check if the required configuration fields are present
	if workflowConfig.ExecutionUrlFormat == "" || workflowConfig.Domain == "" {
		return ""
	}

	tmpl, _ := template.New("url").Parse(workflowConfig.ExecutionUrlFormat)
	var buf bytes.Buffer
	tmpl.Execute(&buf, map[string]string{"Domain": workflowConfig.Domain, "ExecutionID": name})
	return buf.String()
}

// Run executes and monitors the workflow for a pipeline run.
//
// This method handles the complete lifecycle of workflow execution:
//   - Starting new workflows with configured inputs and environment
//   - Monitoring ongoing workflow execution status
//   - Querying and updating task-level progress
//   - Handling workflow termination requests
//   - Managing state transitions based on workflow outcomes
//
// The workflow is executed using Cadence/Temporal with the Starlark workflow type.
// Task progress is continuously queried and reflected in the pipeline run status.
//
// Returns a condition indicating the workflow state (RUNNING, SUCCEEDED, FAILED, KILLED).
func (a *ExecuteWorkflowActor) Run(ctx context.Context, pipelineRun *v2.PipelineRun, previousCondition *apipb.Condition) (*apipb.Condition, error) {
	logger := a.logger.With(zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)))

	// Check for manual retry spec field and process if present
	// Manual retries can work from any workflow state (FAILED, TERMINATED, RUNNING, etc.)
	retryErr := a.processManualRetrySpec(ctx, pipelineRun)
	if retryErr != nil {
		logger.Error("failed to process manual retry spec", zap.Error(retryErr))
		return nil, retryErr
	}

	executeWorkflowStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
	if executeWorkflowStep == nil {
		logger.Info("execute workflow step not found, setting to pending")
		executeWorkflowStep = &v2.PipelineRunStepInfo{
			Name:        pipelinerunutils.ExecuteWorkflowStepName,
			DisplayName: pipelinerunutils.ExecuteWorkflowStepName,
			State:       v2.PIPELINE_RUN_STEP_STATE_PENDING,
			StartTime:   pbtypes.TimestampNow(),
			LogUrl:      a.GetWorkflowUrl(pipelineRun.Name),
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

	// Note: Automatic retry logic is handled at the Starlark task level (ray_task.star, spark/task.star)
	// No workflow-level automatic retries needed here

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

// StartWorkflow initiates a new workflow execution in Cadence/Temporal.
//
// The method prepares workflow inputs from the pipeline specification, including
// args, kwargs, and environment variables. It retrieves the pipeline manifest from
// blob storage and starts the workflow with configured timeouts and task list.
//
// Returns the workflow execution details (ID and RunID) or an error if startup fails.
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

	// Set default UF_STORAGE_URL if not provided in pipeline config
	if pipelineRun.Spec.WorkspaceRootDir != "" {
		envs["UF_STORAGE_URL"] = pipelineRun.Spec.WorkspaceRootDir
	} else {
		envs["UF_STORAGE_URL"] = DefaultWorkSpaceRootURL
	}

	// Apply dynamic parameters from pipelineRun.Spec.Input to override pipeline manifest
	if pipelineConfigMap != nil {
		if _, ok := pipelineConfigMap[WorkflowArgsKey]; ok {
			args = pipelineConfigMap[WorkflowArgsKey].([]interface{})
		}
		if val, ok := pipelineConfigMap[WorkflowKWArgsKey]; ok {
			kwArgs = val.([]interface{})
		}
		if val, ok := pipelineConfigMap[WorkflowEnvironKey]; ok {
			// Merge environment variables instead of replacing them
			// This preserves default values like UF_STORAGE_URL set earlier
			for k, v := range val.(map[string]interface{}) {
				envs[k] = v
			}
		}
	}

	// Apply DevRun environment overrides if present
	if pipelineRun.Spec.Input != nil {
		if pipelineConfigMap == nil {
			pipelineConfigMap = make(map[string]interface{})
		}

		// Override input fields
		applyInputOverrides(pipelineRun.Spec.Input, pipelineConfigMap,
			WorkflowTaskConfigsKey, // Yaml based pipeline
			WorkflowConfigKey,      // Yaml based pipeline
			WorkflowArgsKey,        // Python SDK based pipeline
			WorkflowKWArgsKey,      // Python SDK based pipeline
		)

		// Apply DevRun environment overrides if present
		if environField := pipelineRun.Spec.Input.Fields["environ"]; environField != nil {
			if err := applyDevRunEnvironmentOverrides(envs, environField.GetStructValue()); err != nil {
				return nil, nil, nil, fmt.Errorf("failed to apply DevRun environment overrides: %w", err)
			}
		}
	}

	if pipelineConfigMap != nil {
		args, kwArgs = extractWorkflowInputs(pipelineConfigMap, envs)
	}

	envs["MA_NAMESPACE"] = pipelineRun.Namespace
	envs["MA_PIPELINE_RUN_NAME"] = pipelineRun.Name
	addTaskImageToEnv(pipelineRun, envs)
	return args, kwArgs, envs, nil
}

// extractWorkflowInputs extracts workflow inputs (args and kw_args) from pipeline config.
// It handles both Yaml-based pipelines and Python SDK pipelines.
// For Yaml based pipeline: returns workflow_config and task_configs as args
// For Python SDK based pipeline: processes args > kw_args > environ in order
// Returns args and kwArgs (environ is merged into the provided envs map).
func extractWorkflowInputs(pipelineConfigMap map[string]interface{}, envs map[string]interface{}) ([]interface{}, []interface{}) {
	var args []interface{}
	var kwArgs []interface{}

	if _, ok := pipelineConfigMap[WorkflowTaskConfigsKey]; ok {
		// Yaml based pipeline
		// Return workflow_config and task_configs as positional args
		if pipelineConfigMap[WorkflowConfigKey] != nil {
			args = append(args, pipelineConfigMap[WorkflowConfigKey])
		}
		if pipelineConfigMap[WorkflowTaskConfigsKey] != nil {
			args = append(args, pipelineConfigMap[WorkflowTaskConfigsKey])
		}
		envs["YAML_BASED_PIPELINE"] = "true"
	} else {
		// Python SDK based pipeline
		// Process args, kw_args, and env variables in order
		if _, ok := pipelineConfigMap[WorkflowArgsKey]; ok {
			args = pipelineConfigMap[WorkflowArgsKey].([]interface{})
		} else if val, ok := pipelineConfigMap[WorkflowKWArgsKey]; ok {
			kwArgs = convertKwArgsMapToList(val)
		} else if val, ok := pipelineConfigMap[WorkflowEnvironKey]; ok {
			environMap := val.(map[string]interface{})
			for k, v := range environMap {
				envs[k] = v
			}
		}
	}
	return args, kwArgs
}

// convertKwArgsMapToList converts kw_args from map format to list of [key, value] pairs.
// Starlark workflow input expects kw_args as a list of [key, value] pairs, key as parameter name and value as parameter value.
func convertKwArgsMapToList(val interface{}) []interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		// Convert map to list of [key, value] pairs
		kwArgsList := make([]interface{}, 0, len(v))
		for key, value := range v {
			kwArgsList = append(kwArgsList, []interface{}{key, value})
		}
		return kwArgsList
	case []interface{}:
		// Already in list format
		return v
	default:
		// Unknown format, return empty list
		return []interface{}{}
	}
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

// GetType returns the condition type identifier for this actor.
func (a *ExecuteWorkflowActor) GetType() string {
	return ExecuteWorkflowType
}

// propagateTerminalStateToSubsteps updates substep states when the parent workflow reaches a terminal state.
//
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

		// Parse task progress (now includes activity IDs)
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

		// Merge the task progress
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
	// Make sure we don't overwrite activity ID if it already exists
	if oldStepInfo.ActivityId != "" {
		mergedStepInfo.ActivityId = oldStepInfo.ActivityId
	}

	// Preserve retry-related metadata across merges
	// Retry metadata is maintained in stepInfo.Message for failed tasks
	if oldStepInfo.State == v2.PIPELINE_RUN_STEP_STATE_FAILED && newStepInfo.State == v2.PIPELINE_RUN_STEP_STATE_PENDING {
		// Task is being reset for retry - update state but preserve failure history
		mergedStepInfo.Message = fmt.Sprintf("Retrying task (previous failure: %s)", oldStepInfo.Message)
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
	stepInfo.ActivityId = taskProgress.FirstActivityID

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

	// Handle retry attempt tracking with enhanced metadata
	if taskProgress.RetryAttemptID != "" {
		// Create resource entry for this retry attempt
		attemptResource := &v2.PipelineRunResource{
			Resource: &v2.PipelineRunResource_ExternalResource{
				ExternalResource: &v2.ExternalResource{
					Name: fmt.Sprintf("Attempt%s-DriverURL", taskProgress.RetryAttemptID),
					Url:  taskProgress.TaskLog,
				},
			},
		}

		// Add retry metadata if this is a retry attempt
		if taskProgress.RetryAttemptID != "0" {
			attemptResource.Resource.(*v2.PipelineRunResource_ExternalResource).ExternalResource.Name =
				fmt.Sprintf("Retry-Attempt%s-DriverURL", taskProgress.RetryAttemptID)
		}

		stepInfo.Resources = []*v2.PipelineRunResource{attemptResource}
		stepInfo.AttemptIds = []string{taskProgress.RetryAttemptID}
	}

	return stepInfo
}

// Enhanced functions removed - activity ID functionality has been merged into TaskProgress

// Storage methods removed - now using on-demand querying of task progress for activity IDs

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

func applyInputOverrides(input *pbtypes.Struct, pipelineConfigMap map[string]interface{}, fieldKeys ...string) {
	if input == nil {
		return
	}
	for _, fieldKey := range fieldKeys {
		applyInputFieldToConfigMap(input, fieldKey, pipelineConfigMap)
	}
}

func applyInputFieldToConfigMap(input *pbtypes.Struct, fieldKey string, pipelineConfigMap map[string]interface{}) {
	if field := input.Fields[fieldKey]; field != nil {
		if fieldStruct := field.GetStructValue(); fieldStruct != nil {
			marshaler := &jsonpb.Marshaler{}
			if fieldJSON, err := marshaler.MarshalToString(fieldStruct); err == nil {
				var fieldMap map[string]interface{}
				if err := json.Unmarshal([]byte(fieldJSON), &fieldMap); err == nil {
					pipelineConfigMap[fieldKey] = fieldMap
				}
			}
		}
	}
}

// findTaskResetEventIDByActivityID finds reset boundary using the provided first activity ID
// This is the most precise approach as it uses the actual activity ID from step info
func (a *ExecuteWorkflowActor) findTaskResetEventIDByActivityID(ctx context.Context, workflowID, runID, firstActivityID string) (int64, error) {
	logger := a.logger.With(
		zap.String("workflowID", workflowID),
		zap.String("runID", runID),
		zap.String("firstActivityID", firstActivityID),
	)

	logger.Info("finding reset event using first activity ID")

	// Get workflow history
	history, err := a.workflowClient.GetWorkflowExecutionHistory(ctx, workflowID, runID, nil, 5000)
	if err != nil {
		return 0, fmt.Errorf("failed to get workflow history: %w", err)
	}

	// Find the exact event where the first activity was scheduled
	var firstActivityScheduledEventID int64
	for _, event := range history.Events {
		if event.EventType == a.workflowClient.GetActivityTaskScheduledEventType() {
			if activityID, ok := event.Details["activity_id"].(string); ok {
				if activityID == firstActivityID {
					firstActivityScheduledEventID = event.EventID
					logger.Info("found first activity scheduled event",
						zap.Int64("eventID", event.EventID))
					break
				}
			}
		}
	}

	if firstActivityScheduledEventID == 0 {
		return 0, fmt.Errorf("could not find scheduled event for first activity %s", firstActivityID)
	}

	// Find the decision/workflow task completed event immediately before the first activity
	var resetEventID int64
	for i := len(history.Events) - 1; i >= 0; i-- {
		event := history.Events[i]

		// Stop when we reach the first activity scheduled event
		if event.EventID >= firstActivityScheduledEventID {
			continue
		}

		// Look for the decision/activity task completed event just before
		if event.EventType == a.workflowClient.GetActivityTaskCompletedEventType() || event.EventType == a.workflowClient.GetDecisionTaskCompletedEventType() {
			resetEventID = event.EventID
			break
		}
	}

	if resetEventID == 0 {
		return 0, fmt.Errorf("could not find safe reset boundary before first activity %s", firstActivityID)
	}

	logger.Info("found precise reset boundary using activity ID",
		zap.Int64("firstActivityScheduledEventID", firstActivityScheduledEventID),
		zap.Int64("resetEventID", resetEventID))

	return resetEventID, nil
}

// processManualRetrySpec checks for manual retry spec field and triggers retry if present
func (a *ExecuteWorkflowActor) processManualRetrySpec(ctx context.Context, pipelineRun *v2.PipelineRun) error {
	logger := a.logger.With(
		zap.String("pipelineRun", fmt.Sprintf("%s/%s", pipelineRun.Namespace, pipelineRun.Name)),
	)

	// Check if retry info field is set
	retryInfo := pipelineRun.Spec.RetryInfo
	if retryInfo == nil || retryInfo.ActivityId == "" {
		return nil
	}

	// New trigger condition: only process if workflowRunId differs from current status
	// This prevents duplicate processing and ensures precise retry control
	if retryInfo.WorkflowRunId != "" && retryInfo.WorkflowRunId == pipelineRun.Status.WorkflowRunId {
		logger.Info("processing retry - workflowRunId differs from current status",
			zap.String("retryWorkflowRunId", retryInfo.WorkflowRunId),
			zap.String("currentWorkflowRunId", pipelineRun.Status.WorkflowRunId),
		)
	} else {
		logger.Debug("skipping retry processing - workflowRunId matches current status or is empty",
			zap.String("retryWorkflowRunId", retryInfo.WorkflowRunId),
			zap.String("currentWorkflowRunId", pipelineRun.Status.WorkflowRunId),
		)
		return nil
	}

	logger.Info("manual retry spec field detected",
		zap.String("activityId", retryInfo.ActivityId),
		zap.String("workflowId", retryInfo.WorkflowId),
		zap.String("workflowRunId", retryInfo.WorkflowRunId),
		zap.String("reason", retryInfo.Reason),
	)

	// Use the activity ID directly from retryInfo for precise reset boundary
	activityID := retryInfo.ActivityId

	// Find reset event ID using the queried activity ID
	resetEventID, err := a.findTaskResetEventIDByActivityID(ctx, pipelineRun.Status.WorkflowId, pipelineRun.Status.WorkflowRunId, activityID)
	if err != nil {
		logger.Error("failed to find reset event ID",
			zap.String("activityId", retryInfo.ActivityId),
			zap.String("activityID", activityID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to find reset event ID for activity %s: %w", retryInfo.ActivityId, err)
	}

	// Perform workflow reset
	resetOptions := clientInterfaces.ResetWorkflowOptions{
		WorkflowID: pipelineRun.Status.WorkflowId,
		RunID:      pipelineRun.Status.WorkflowRunId,
		EventID:    resetEventID,
		Reason:     fmt.Sprintf("Manual retry for activity: %s - %s", retryInfo.ActivityId, retryInfo.Reason),
		RequestID:  fmt.Sprintf("manual-retry-%s-%d", pipelineRun.Name, time.Now().Unix()),
	}

	newWorkflowRun, err := a.workflowClient.ResetWorkflow(ctx, resetOptions)
	if err != nil {
		logger.Error("workflow reset failed",
			zap.String("activityId", retryInfo.ActivityId),
			zap.String("workflowId", pipelineRun.Status.WorkflowId),
			zap.String("workflowRunId", pipelineRun.Status.WorkflowRunId),
			zap.Int64("resetEventID", resetEventID),
			zap.Error(err),
		)
		return fmt.Errorf("workflow reset failed for activity %s: %w", retryInfo.ActivityId, err)
	}

	// Update pipeline run status
	pipelineRun.Status.State = v2.PIPELINE_RUN_STATE_RUNNING
	pipelineRun.Status.WorkflowRunId = newWorkflowRun.RunID

	// Update execute workflow step state from FAILED/KILLED to RUNNING for retry
	executeWorkflowStep := pipelinerunutils.GetStep(pipelineRun, pipelinerunutils.ExecuteWorkflowStepName)
	if executeWorkflowStep != nil &&
		(executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_SUCCEEDED ||
			executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_FAILED ||
			executeWorkflowStep.State == v2.PIPELINE_RUN_STEP_STATE_KILLED) {
		logger.Info("updating execute workflow step state to RUNNING for retry",
			zap.String("previousState", executeWorkflowStep.State.String()))
		executeWorkflowStep.State = v2.PIPELINE_RUN_STEP_STATE_RUNNING
		executeWorkflowStep.Message = fmt.Sprintf("Retry started for activity: %s", retryInfo.ActivityId)
	}

	// Clear kill flag from previous run to prevent immediate termination of new workflow
	if pipelineRun.Spec.Kill {
		logger.Info("clearing kill flag from previous run for retry")
		pipelineRun.Spec.Kill = false
	}

	return nil
}
