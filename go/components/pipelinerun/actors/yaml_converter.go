package actors

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// YAMLToUniflowConverter handles conversion from DAG Factory YAML to Uniflow tar
type YAMLToUniflowConverter struct {
	logger    *zap.Logger
	taskTypes map[string]string // Map task names to their types (notebook, ray, spark, etc.)
}

// NewYAMLToUniflowConverter creates a new converter instance
func NewYAMLToUniflowConverter(logger *zap.Logger) *YAMLToUniflowConverter {
	return &YAMLToUniflowConverter{
		logger:    logger.With(zap.String("component", "yaml_converter")),
		taskTypes: make(map[string]string),
	}
}

// Pipeline Specification Models (New format)

type TriggerSettings struct {
	Mode           string `yaml:"mode"`                      // MANUAL, SCHEDULED, EVENT_BASED
	Crontab        string `yaml:"crontab,omitempty"`         // cron expression
	StartDate      string `yaml:"start_date,omitempty"`      // start date
	EndDate        string `yaml:"end_date,omitempty"`        // end date
	Catchup        string `yaml:"catchup,omitempty"`         // catchup setting
	MaxConcurrency int    `yaml:"max_concurrency,omitempty"` // max concurrent runs
}

type SlackConfig struct {
	Channel    string `yaml:"channel,omitempty"`
	UserOncall string `yaml:"user_oncall,omitempty"`
}

type SplunkConfig struct {
	Oncall string `yaml:"oncall,omitempty"`
}

type NotificationConfig struct {
	SlackConfig  *SlackConfig  `yaml:"slack_config,omitempty"`
	SplunkConfig *SplunkConfig `yaml:"splunk_config,omitempty"`
}

type PipelineArtifacts struct {
	Location string `yaml:"location,omitempty"`
}

type PipelineConfig struct {
	ID                string              `yaml:"id"`
	Type              string              `yaml:"type"`
	Name              string              `yaml:"name"`
	Owner             string              `yaml:"owner"`
	CreatedBy         string              `yaml:"created_by"`
	CreatedTime       int64               `yaml:"created_time"`
	Workspace         string              `yaml:"workspace"`
	TriggerSettings   *TriggerSettings    `yaml:"trigger_settings,omitempty"`
	Schedule          *ScheduleConfig     `yaml:"schedule,omitempty"`
	SLA               string              `yaml:"sla,omitempty"`
	Timeout           string              `yaml:"timeout,omitempty"`
	Notifications     *NotificationConfig `yaml:"notifications,omitempty"`
	Description       string              `yaml:"description,omitempty"`
	Tags              []string            `yaml:"tags,omitempty"`
	MetadataTags      []string            `yaml:"metadata_tags,omitempty"`
	Tasks             []*TaskSpec         `yaml:"tasks,omitempty"`
	PipelineArtifacts *PipelineArtifacts  `yaml:"pipeline_artifacts,omitempty"`
}

type NotebookTaskConfig struct {
	NotebookPath   string                 `yaml:"notebook_path"`
	TaskParameters map[string]interface{} `yaml:"task_parameters,omitempty"`
	UserParameters map[string]interface{} `yaml:"user_parameters,omitempty"`
}

type ForEachTaskConfig struct {
	Inputs       string              `yaml:"inputs"` // String template reference
	Concurrency  int                 `yaml:"concurrency,omitempty"`
	NotebookTask *NotebookTaskConfig `yaml:"notebook_task,omitempty"`
	RayTask      *RayTaskConfig      `yaml:"ray_task,omitempty"`
	SparkTask    *SparkTaskConfig    `yaml:"spark_task,omitempty"`
	SparkOneTask *SparkOneTaskConfig `yaml:"sparkone_task,omitempty"`
}

type ConditionConfig struct {
	Op    string `yaml:"op"`    // EQUAL_TO, NOT_EQUAL_TO, etc.
	Left  string `yaml:"left"`  // Left side of comparison
	Right string `yaml:"right"` // Right side of comparison
}

type IfElseTaskConfig struct {
	Inputs    string           `yaml:"inputs"`
	Condition *ConditionConfig `yaml:"condition,omitempty"`
}

type SparkOneTaskConfig struct {
	Retries        int                    `yaml:"retries,omitempty"`
	RetryDelay     string                 `yaml:"retry_delay,omitempty"`
	TaskParameters map[string]interface{} `yaml:"task_parameters,omitempty"`
	UserParameters map[string]interface{} `yaml:"user_parameters,omitempty"`
}

type DependsOnConfig struct {
	TaskID  string `yaml:"task_name"` // Changed to match actual YAML structure
	Outcome string `yaml:"outcome"`
}

type ResourceConfig struct {
	CPU               int    `yaml:"cpu,omitempty"`
	Memory            string `yaml:"memory,omitempty"`
	GPU               int    `yaml:"gpu,omitempty"`
	WorkerInstances   int    `yaml:"worker_instances,omitempty"`
	DriverCPU         int    `yaml:"driver_cpu,omitempty"`
	DriverMemory      string `yaml:"driver_memory,omitempty"`
	ExecutorCores     int    `yaml:"executor_cores,omitempty"`
	ExecutorInstances int    `yaml:"executor_instances,omitempty"`
	ExecutorMemory    string `yaml:"executor_memory,omitempty"`
}

type RayTaskConfig struct {
	TaskParameters map[string]interface{} `yaml:"task_parameters,omitempty"`
	UserParameters map[string]interface{} `yaml:"user_parameters,omitempty"`
}

type SparkTaskConfig struct {
	TaskParameters map[string]interface{} `yaml:"task_parameters,omitempty"`
	UserParameters map[string]interface{} `yaml:"user_parameters,omitempty"`
}

type TaskSpec struct {
	TaskID   string `yaml:"task_id"`   // Keep original field mapping
	TaskName string `yaml:"task_name"` // Add support for new field mapping

	// Task type configs (mutually exclusive)
	NotebookTask *NotebookTaskConfig `yaml:"notebook_task,omitempty"`
	ForEachTask  *ForEachTaskConfig  `yaml:"for_each_task,omitempty"`
	IfElseTask   *IfElseTaskConfig   `yaml:"if_else_task,omitempty"`
	RayTask      *RayTaskConfig      `yaml:"ray_task,omitempty"`
	SparkTask    *SparkTaskConfig    `yaml:"spark_task,omitempty"`
	SparkOneTask *SparkOneTaskConfig `yaml:"sparkone_task,omitempty"`

	// Dependencies
	DependsOn []*DependsOnConfig `yaml:"depends_on,omitempty"`
}

// GetEffectiveTaskID returns the task ID, preferring task_name over task_id
func (t *TaskSpec) GetEffectiveTaskID() string {
	if t.TaskName != "" {
		return t.TaskName
	}
	return t.TaskID
}

type ScheduleConfig struct {
	StartDate      string `yaml:"start_date,omitempty"`
	MaxConcurrency int    `yaml:"max_concurrency,omitempty"`
	Timeout        string `yaml:"timeout,omitempty"`
}

type DAGFactorySpec struct {
	SpecVersion string                 `yaml:"spec_version"`
	Pipeline    *PipelineConfig        `yaml:"pipeline"`
	Tasks       []*TaskSpec            `yaml:"tasks,omitempty"`
	Parameters  map[string]interface{} `yaml:"parameters,omitempty"`
}

// ConvertYAMLToUniflowTar converts DAG Factory YAML content to Uniflow tar bytes
func (c *YAMLToUniflowConverter) ConvertYAMLToUniflowTar(ctx context.Context, pipeline *v2.Pipeline) ([]byte, error) {
	log := c.logger.With(zap.String("pipeline", pipeline.Name), zap.String("namespace", pipeline.Namespace))
	log.Info("Converting YAML pipeline to Uniflow tar")

	// Extract YAML content from pipeline manifest
	yamlContent, err := c.extractYAMLContent(pipeline.Spec.Manifest.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract YAML content: %w", err)
	}

	log.Debug("Extracted YAML content", zap.Int("length", len(yamlContent)))

	// Parse DAG Factory YAML
	var dagSpec DAGFactorySpec
	if unmarshalErr := yaml.Unmarshal([]byte(yamlContent), &dagSpec); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse DAG Factory YAML: %w", unmarshalErr)
	}

	// Get consolidated task list from either pipeline.tasks or top-level tasks
	allTasks := c.getConsolidatedTasks(&dagSpec)
	log.Info("Parsed DAG Factory spec", zap.String("pipeline_name", dagSpec.Pipeline.Name), zap.Int("tasks", len(allTasks)))

	// Convert to Uniflow format
	uniflowYAML, err := c.convertToUniflowYAML(&dagSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to Uniflow YAML: %w", err)
	}

	// Generate Starlark workflow code
	starlarkCode, err := c.generateStarlarkCode(&dagSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Starlark workflow: %w", err)
	}

	// Create tar archive
	tarBytes, err := c.createUniflowTar(uniflowYAML, starlarkCode, &dagSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}

	log.Info("Successfully converted to Uniflow tar", zap.Int("tar_size", len(tarBytes)))
	return tarBytes, nil
}

// extractYAMLContent extracts YAML content from google.protobuf.Any field
func (c *YAMLToUniflowConverter) extractYAMLContent(contentAny *types.Any) (string, error) {
	if contentAny == nil {
		return "", fmt.Errorf("no content provided")
	}

	// Check if it's a BytesValue
	if contentAny.GetTypeUrl() == "type.googleapis.com/google.protobuf.BytesValue" {
		c.logger.Debug("Processing BytesValue",
			zap.String("type_url", contentAny.GetTypeUrl()),
			zap.Int("raw_value_length", len(contentAny.GetValue())))

		// Try to unmarshal as proper protobuf first
		var bytesValue types.BytesValue
		if err := types.UnmarshalAny(contentAny, &bytesValue); err != nil {
			c.logger.Debug("Failed to unmarshal as protobuf BytesValue, trying JSON format", zap.Error(err))

			// If that fails, try to parse as JSON format from Kubernetes YAML
			return c.extractFromJSONFormat(contentAny)
		}

		base64Content := string(bytesValue.Value)

		c.logger.Debug("Extracting YAML from protobuf BytesValue",
			zap.Int("base64_length", len(base64Content)),
			zap.String("base64_preview", func() string {
				if len(base64Content) > 50 {
					return base64Content[:50]
				}
				return base64Content
			}()))

		// Check if the content is already YAML (not base64)
		if strings.HasPrefix(base64Content, "spec_version:") ||
			strings.HasPrefix(base64Content, "# ") ||
			strings.Contains(base64Content, "pipeline:") {
			c.logger.Debug("Content appears to be raw YAML in BytesValue, using directly")
			return base64Content, nil
		}

		decoded, decodeErr := base64.StdEncoding.DecodeString(base64Content)
		if decodeErr != nil {
			c.logger.Debug("Base64 decode failed, checking if content is raw YAML", zap.Error(decodeErr))
			// Maybe it's raw YAML stored directly
			if strings.Contains(base64Content, "spec_version:") {
				c.logger.Debug("Found YAML markers in failed base64 content, using as raw YAML")
				return base64Content, nil
			}
			return "", fmt.Errorf("failed to decode base64 content: %w", decodeErr)
		}

		c.logger.Debug("Successfully decoded YAML content", zap.Int("decoded_length", len(decoded)))
		return string(decoded), nil
	}

	return "", fmt.Errorf("unsupported content type: %s", contentAny.GetTypeUrl())
}

// extractFromJSONFormat handles BytesValue stored in JSON format from Kubernetes YAML
func (c *YAMLToUniflowConverter) extractFromJSONFormat(contentAny *types.Any) (string, error) {
	// Parse the JSON content from the Any value
	valueBytes := contentAny.GetValue()

	c.logger.Debug("Attempting to parse BytesValue from JSON format",
		zap.Int("value_length", len(valueBytes)),
		zap.String("value_preview", func() string {
			if len(valueBytes) > 100 {
				return string(valueBytes[:100])
			}
			return string(valueBytes)
		}()))

	// First try to parse as JSON like: {"value":"base64content"}
	var jsonValue struct {
		Value string `json:"value"`
	}

	if err := json.Unmarshal(valueBytes, &jsonValue); err != nil {
		// If JSON parsing fails, check if it's already decoded YAML content
		c.logger.Debug("Failed to parse as JSON, checking if it's raw YAML", zap.Error(err))

		yamlContent := string(valueBytes)

		// Check if it looks like YAML content (starts with spec_version or similar)
		if strings.HasPrefix(yamlContent, "spec_version:") ||
			strings.HasPrefix(yamlContent, "# ") ||
			strings.Contains(yamlContent, "pipeline:") {
			c.logger.Debug("Content appears to be raw YAML, using directly")
			return yamlContent, nil
		}

		// Try base64 decode as last resort
		decoded, decodeErr := base64.StdEncoding.DecodeString(yamlContent)
		if decodeErr != nil {
			return "", fmt.Errorf("failed to parse as JSON BytesValue, not raw YAML, and failed base64 decode: JSON error: %v, base64 error: %v", err, decodeErr)
		}

		c.logger.Debug("Successfully decoded YAML content from direct base64", zap.Int("decoded_length", len(decoded)))
		return string(decoded), nil
	}

	if jsonValue.Value == "" {
		return "", fmt.Errorf("empty value in BytesValue JSON")
	}

	c.logger.Debug("Extracting YAML from JSON BytesValue",
		zap.Int("base64_length", len(jsonValue.Value)),
		zap.String("base64_preview", func() string {
			if len(jsonValue.Value) > 50 {
				return jsonValue.Value[:50]
			}
			return jsonValue.Value
		}()))

	decoded, decodeErr := base64.StdEncoding.DecodeString(jsonValue.Value)
	if decodeErr != nil {
		return "", fmt.Errorf("failed to decode base64 content from JSON: %w", decodeErr)
	}

	c.logger.Debug("Successfully decoded YAML content from JSON", zap.Int("decoded_length", len(decoded)))
	return string(decoded), nil
}

// convertToUniflowYAML converts DAG Factory spec to Uniflow YAML format
func (c *YAMLToUniflowConverter) convertToUniflowYAML(dagSpec *DAGFactorySpec) (string, error) {
	// Get consolidated task list from either pipeline.tasks or top-level tasks
	allTasks := c.getConsolidatedTasks(dagSpec)

	// Create Uniflow workflow config
	uniflowConfig := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":        dagSpec.Pipeline.Name,
			"version":     dagSpec.SpecVersion,
			"description": dagSpec.Pipeline.Description,
			"author":      dagSpec.Pipeline.CreatedBy,
		},
		"defaults": map[string]interface{}{
			"image_spec":     "michelangelo-base:latest",
			"cache_enabled":  true,
			"retry_attempts": 0,
		},
		"environment": c.convertEnvironment(dagSpec.Pipeline),
	}

	// Convert tasks - fail if any task conversion fails
	tasks, err := c.convertTasks(allTasks)
	if err != nil {
		return "", fmt.Errorf("failed to convert tasks: %w", err)
	}
	uniflowConfig["tasks"] = tasks

	// Add storage URL from workspace
	if dagSpec.Pipeline.Workspace != "" && strings.HasPrefix(dagSpec.Pipeline.Workspace, "component:") {
		componentName := strings.TrimPrefix(dagSpec.Pipeline.Workspace, "component:")
		uniflowConfig["defaults"].(map[string]interface{})["storage_url"] = fmt.Sprintf("s3://uber-ml-workflows/%s", componentName)
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(uniflowConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Uniflow config: %w", err)
	}

	return string(yamlBytes), nil
}

// convertEnvironment converts pipeline config to Uniflow environment
func (c *YAMLToUniflowConverter) convertEnvironment(pipeline *PipelineConfig) map[string]interface{} {
	env := map[string]interface{}{
		"variables": map[string]interface{}{
			"PIPELINE_ID":        pipeline.ID,
			"PIPELINE_NAME":      pipeline.Name,
			"PIPELINE_WORKSPACE": pipeline.Workspace,
		},
	}

	// Add trigger settings information
	if pipeline.TriggerSettings != nil {
		triggers := pipeline.TriggerSettings
		if triggers.Mode != "" {
			env["variables"].(map[string]interface{})["TRIGGER_MODE"] = triggers.Mode
		}
		if triggers.Crontab != "" {
			env["variables"].(map[string]interface{})["CRONTAB"] = triggers.Crontab
		}
		if triggers.MaxConcurrency > 0 {
			env["variables"].(map[string]interface{})["MAX_CONCURRENCY"] = fmt.Sprintf("%d", triggers.MaxConcurrency)
		}
	}

	// Add pipeline-level SLA and timeout
	if pipeline.SLA != "" {
		env["variables"].(map[string]interface{})["SLA"] = pipeline.SLA
	}
	if pipeline.Timeout != "" {
		env["variables"].(map[string]interface{})["TIMEOUT"] = pipeline.Timeout
	}

	// Add notification config
	if pipeline.Notifications != nil {
		notifications := pipeline.Notifications
		if notifications.SlackConfig != nil && notifications.SlackConfig.Channel != "" {
			env["variables"].(map[string]interface{})["SLACK_CHANNEL"] = notifications.SlackConfig.Channel
		}
		if notifications.SplunkConfig != nil && notifications.SplunkConfig.Oncall != "" {
			env["variables"].(map[string]interface{})["SPLUNK_ONCALL"] = notifications.SplunkConfig.Oncall
		}
	}

	return env
}

// convertTasks converts DAG Factory tasks to Uniflow format
func (c *YAMLToUniflowConverter) convertTasks(tasks []*TaskSpec) (map[string]interface{}, error) {
	uniflowTasks := make(map[string]interface{})

	for _, task := range tasks {
		uniflowTask, err := c.convertSingleTask(task)
		if err != nil {
			// Return error instead of silently failing
			return nil, err
		}
		uniflowTasks[task.GetEffectiveTaskID()] = uniflowTask
	}

	return uniflowTasks, nil
}

// convertSingleTask converts a single DAG Factory task to Uniflow format
func (c *YAMLToUniflowConverter) convertSingleTask(task *TaskSpec) (map[string]interface{}, error) {
	uniflowTask := map[string]interface{}{
		"cache_enabled":  true,
		"retry_attempts": 3,
	}

	// Convert task type
	if task.NotebookTask != nil {
		uniflowTask["function"] = "examples.notebook_workflow.executor.notebook_executor"
		uniflowTask["config"] = map[string]interface{}{"type": "NotebookTask"}
		uniflowTask["inputs"] = map[string]interface{}{
			"notebook_path":   task.NotebookTask.NotebookPath,
			"user_parameters": task.NotebookTask.UserParameters,
		}
	} else if task.ForEachTask != nil {
		// Determine the function and config based on the inner task type
		if task.ForEachTask.NotebookTask != nil {
			uniflowTask["function"] = "examples.notebook_workflow.executor.notebook_executor"
			uniflowTask["config"] = map[string]interface{}{"type": "NotebookTask"}
			uniflowTask["inputs"] = map[string]interface{}{
				"notebook_path":   task.ForEachTask.NotebookTask.NotebookPath,
				"user_parameters": task.ForEachTask.NotebookTask.UserParameters,
			}
		} else if task.ForEachTask.RayTask != nil {
			// Extract function from ray task parameters - REQUIRED
			fn, exists := task.ForEachTask.RayTask.TaskParameters["function"]
			if !exists {
				return nil, fmt.Errorf("ray_task in for_each_task '%s' must specify 'function' in task_parameters", task.TaskID)
			}
			uniflowTask["function"] = fmt.Sprintf("%v", fn)
			uniflowTask["config"] = map[string]interface{}{"type": "RayTask"}
			uniflowTask["inputs"] = task.ForEachTask.RayTask.UserParameters
		} else if task.ForEachTask.SparkTask != nil {
			// Extract function from spark task parameters - REQUIRED
			fn, exists := task.ForEachTask.SparkTask.TaskParameters["function"]
			if !exists {
				return nil, fmt.Errorf("spark_task in for_each_task '%s' must specify 'function' in task_parameters", task.TaskID)
			}
			uniflowTask["function"] = fmt.Sprintf("%v", fn)
			uniflowTask["config"] = map[string]interface{}{"type": "SparkTask"}
			uniflowTask["inputs"] = task.ForEachTask.SparkTask.UserParameters
		} else if task.ForEachTask.SparkOneTask != nil {
			// Extract function from sparkone task parameters - REQUIRED
			fn, exists := task.ForEachTask.SparkOneTask.TaskParameters["function"]
			if !exists {
				return nil, fmt.Errorf("sparkone_task in for_each_task '%s' must specify 'function' in task_parameters", task.TaskID)
			}
			uniflowTask["function"] = fmt.Sprintf("%v", fn)
			uniflowTask["config"] = map[string]interface{}{"type": "SparkOneTask"}
			uniflowTask["inputs"] = task.ForEachTask.SparkOneTask.UserParameters
		} else {
			// No inner task type specified - this is an error
			return nil, fmt.Errorf("for_each_task '%s' must specify one of: notebook_task, ray_task, spark_task, or sparkone_task", task.GetEffectiveTaskID())
		}

		uniflowTask["expand"] = map[string]interface{}{
			"item": task.ForEachTask.Inputs,
		}
		if task.ForEachTask.Concurrency > 0 {
			uniflowTask["expand"].(map[string]interface{})["max_parallel"] = task.ForEachTask.Concurrency
		}
	} else if task.IfElseTask != nil {
		// Handle if_else_task - this is a conditional evaluation task
		uniflowTask["function"] = "builtin.if_else"
		uniflowTask["config"] = map[string]interface{}{"type": "IfElseTask"}
		uniflowTask["inputs"] = map[string]interface{}{
			"inputs":    task.IfElseTask.Inputs,
			"condition": task.IfElseTask.Condition,
		}
	} else if task.RayTask != nil {
		// Extract function from ray task parameters - REQUIRED
		fn, exists := task.RayTask.TaskParameters["function"]
		if !exists {
			return nil, fmt.Errorf("ray_task '%s' must specify 'function' in task_parameters", task.TaskID)
		}
		uniflowTask["function"] = fmt.Sprintf("%v", fn)
		uniflowTask["config"] = map[string]interface{}{"type": "RayTask"}
		uniflowTask["inputs"] = task.RayTask.UserParameters
	} else if task.SparkTask != nil {
		// Extract function from spark task parameters - REQUIRED
		fn, exists := task.SparkTask.TaskParameters["function"]
		if !exists {
			return nil, fmt.Errorf("spark_task '%s' must specify 'function' in task_parameters", task.TaskID)
		}
		uniflowTask["function"] = fmt.Sprintf("%v", fn)
		uniflowTask["config"] = map[string]interface{}{"type": "SparkTask"}
		uniflowTask["inputs"] = task.SparkTask.UserParameters
	} else if task.SparkOneTask != nil {
		// Extract function from sparkone task parameters - REQUIRED
		fn, exists := task.SparkOneTask.TaskParameters["function"]
		if !exists {
			return nil, fmt.Errorf("sparkone_task '%s' must specify 'function' in task_parameters", task.TaskID)
		}
		uniflowTask["function"] = fmt.Sprintf("%v", fn)
		uniflowTask["config"] = map[string]interface{}{"type": "SparkOneTask"}
		uniflowTask["inputs"] = task.SparkOneTask.UserParameters
	} else {
		return nil, fmt.Errorf("task '%s' must specify one of: notebook_task, for_each_task, if_else_task, ray_task, spark_task, or sparkone_task", task.GetEffectiveTaskID())
	}

	// Convert dependencies
	if len(task.DependsOn) > 0 {
		dependencies, conditions := c.convertDependencies(task.DependsOn)
		if len(dependencies) > 0 {
			uniflowTask["dependencies"] = dependencies
		}
		if conditions != "" {
			uniflowTask["when"] = conditions
		}
	}

	return uniflowTask, nil
}

// convertDependencies converts depends_on to dependencies and conditions
func (c *YAMLToUniflowConverter) convertDependencies(dependsOn []*DependsOnConfig) ([]string, string) {
	var dependencies []string
	var conditions []string

	for _, dep := range dependsOn {
		dependencies = append(dependencies, dep.TaskID)

		// Create condition for outcome
		if dep.Outcome != "" {
			condition := fmt.Sprintf("{{tasks.%s.output.outcome == '%s'}}", dep.TaskID, dep.Outcome)
			conditions = append(conditions, condition)
		}
	}

	// Combine conditions with AND
	var combinedCondition string
	if len(conditions) == 1 {
		combinedCondition = conditions[0]
	} else if len(conditions) > 1 {
		combinedCondition = strings.Join(conditions, " && ")
	}

	return dependencies, combinedCondition
}

// convertTemplate converts {{}} template syntax to + references
func (c *YAMLToUniflowConverter) convertTemplate(templateStr string) string {
	if templateStr == "" || !strings.HasPrefix(templateStr, "{{") {
		return templateStr
	}

	// Remove {{ and }}
	inner := strings.Trim(templateStr, "{}")
	inner = strings.TrimSpace(inner)

	// Handle task references
	if strings.HasPrefix(inner, "tasks.") {
		// Extract task name: tasks.my_task_1.output -> my_task_1
		parts := strings.Split(inner, ".")
		if len(parts) >= 2 {
			taskName := parts[1]
			return fmt.Sprintf("+%s", taskName)
		}
	}

	// Handle pipeline references (convert to environment variables)
	if strings.HasPrefix(inner, "pipeline.") {
		envVar := strings.ToUpper(strings.ReplaceAll(inner, ".", "_"))
		return fmt.Sprintf("${ENV_%s}", envVar)
	}

	// Handle item references (for for_each)
	if inner == "item" {
		return "{{item}}" // Keep as-is for expand pattern
	}

	return templateStr
}

// convertForEachInputsForStarlark converts ForEach inputs string to Starlark format
func (c *YAMLToUniflowConverter) convertForEachInputsForStarlark(inputs string) string {
	if inputs == "" {
		return "[]"
	}

	// Check if it's a template reference
	if strings.Contains(inputs, "{{tasks.") {
		return c.convertStarlarkTemplateReference(inputs)
	}

	// Return as quoted string if not a template
	return fmt.Sprintf("\"%s\"", inputs)
}

// generateStarlarkCode generates the Starlark workflow code directly
func (c *YAMLToUniflowConverter) generateStarlarkCode(dagSpec *DAGFactorySpec) (string, error) {
	var starlarkCode strings.Builder

	// Get consolidated task list from either pipeline.tasks or top-level tasks
	allTasks := c.getConsolidatedTasks(dagSpec)

	// Build task types map for template reference resolution
	c.taskTypes = make(map[string]string)
	for _, task := range allTasks {
		taskName := task.GetEffectiveTaskID()
		if task.NotebookTask != nil {
			c.taskTypes[taskName] = "notebook"
		} else if task.RayTask != nil {
			c.taskTypes[taskName] = "ray"
		} else if task.SparkTask != nil {
			c.taskTypes[taskName] = "spark"
		} else if task.IfElseTask != nil {
			c.taskTypes[taskName] = "if_else"
		} else if task.ForEachTask != nil {
			c.taskTypes[taskName] = "foreach"
		}
	}

	// Generate load statements for required plugins
	loadStatements, err := c.generateStarlarkLoadStatements(allTasks)
	if err != nil {
		return "", fmt.Errorf("failed to generate load statements: %w", err)
	}
	if len(loadStatements) > 0 {
		starlarkCode.WriteString(loadStatements)
		starlarkCode.WriteString("\n")
	}

	// Generate main workflow function
	workflowFunc, err := c.generateStarlarkWorkflowFunction(dagSpec)
	if err != nil {
		return "", fmt.Errorf("failed to generate workflow function: %w", err)
	}
	starlarkCode.WriteString(workflowFunc)

	return starlarkCode.String(), nil
}

// generateStarlarkLoadStatements generates load statements for required plugins
func (c *YAMLToUniflowConverter) generateStarlarkLoadStatements(tasks []*TaskSpec) (string, error) {
	var loadStatements strings.Builder
	usedPlugins := make(map[string]bool)

	// Map task types to plugin info - using relative paths to bundled star files
	taskTypeToPlugin := map[string]struct {
		alias    string
		loadPath string
		function string
	}{
		//"NotebookTask": {"__notebook_task__", "notebook_task.star", "task"},
		"RayTask":   {"__ray_task__", "ray_task.star", "task"},
		"SparkTask": {"__spark_task__", "spark_task.star", "spark_task"},
	}

	// Determine which plugins are needed
	for _, task := range tasks {
		var taskType string
		if task.NotebookTask != nil {
			taskType = "RayTask" // Notebook tasks use ray task infrastructure
		} else if task.RayTask != nil {
			taskType = "RayTask"
		} else if task.SparkTask != nil {
			taskType = "SparkTask"
		} else if task.IfElseTask != nil {
			// If-else tasks don't require any plugins, skip adding load statement
			continue
		} else if task.ForEachTask != nil {
			// ForEach tasks use the inner task type for load statements
			if task.ForEachTask.NotebookTask != nil {
				taskType = "RayTask" // Notebook tasks in ForEach use ray task infrastructure
			} else if task.ForEachTask.SparkTask != nil {
				taskType = "SparkTask"
			} else if task.ForEachTask.RayTask != nil {
				taskType = "RayTask"
			} else if task.ForEachTask.SparkOneTask != nil {
				taskType = "SparkTask" // Use SparkTask for SparkOne
			} else {
				taskType = "RayTask" // Default for ForEach
			}
		} else {
			taskType = "RayTask" // Default to RayTask
		}

		if pluginInfo, exists := taskTypeToPlugin[taskType]; exists && !usedPlugins[taskType] {
			loadStatements.WriteString(fmt.Sprintf("load('/%s', %s='%s')\n",
				pluginInfo.loadPath, pluginInfo.alias, pluginInfo.function))
			usedPlugins[taskType] = true
		}
	}

	return loadStatements.String(), nil
}

// generateStarlarkWorkflowFunction generates the main Starlark workflow function
func (c *YAMLToUniflowConverter) generateStarlarkWorkflowFunction(dagSpec *DAGFactorySpec) (string, error) {
	var workflowCode strings.Builder

	// Get consolidated task list from either pipeline.tasks or top-level tasks
	allTasks := c.getConsolidatedTasks(dagSpec)

	// Function header
	workflowName := c.safeStarlarkName(dagSpec.Pipeline.Name)
	workflowCode.WriteString(fmt.Sprintf("def %s():\n", workflowName))

	if dagSpec.Pipeline.Description != "" {
		workflowCode.WriteString(fmt.Sprintf("    \"\"\"%s\"\"\"\n", dagSpec.Pipeline.Description))
	}

	// Get execution order (topological sort of dependencies)
	executionOrder, err := c.getTaskExecutionOrder(allTasks)
	if err != nil {
		return "", fmt.Errorf("failed to determine execution order: %w", err)
	}

	// Generate task execution code
	for _, task := range executionOrder {
		taskCode, err := c.generateStarlarkTaskExecution(task)
		if err != nil {
			return "", fmt.Errorf("failed to generate task execution for %s: %w", task.GetEffectiveTaskID(), err)
		}

		// Add indentation and task code
		lines := strings.Split(strings.TrimSpace(taskCode), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				workflowCode.WriteString(fmt.Sprintf("    %s\n", line))
			} else {
				workflowCode.WriteString("\n")
			}
		}
	}

	// Return results from all tasks
	var resultItems []string
	for _, task := range executionOrder {
		effectiveID := task.GetEffectiveTaskID()
		safeName := c.safeStarlarkName(effectiveID)
		resultItems = append(resultItems, fmt.Sprintf("\"%s\": %s_result", effectiveID, safeName))
	}

	if len(resultItems) > 0 {
		workflowCode.WriteString(fmt.Sprintf("    return {%s}\n", strings.Join(resultItems, ", ")))
	}

	return workflowCode.String(), nil
}

// getTaskExecutionOrder determines the execution order based on dependencies
func (c *YAMLToUniflowConverter) getTaskExecutionOrder(tasks []*TaskSpec) ([]*TaskSpec, error) {
	// Build dependency graph
	taskMap := make(map[string]*TaskSpec)
	dependencies := make(map[string][]string)

	for _, task := range tasks {
		effectiveID := task.GetEffectiveTaskID()
		taskMap[effectiveID] = task
		var deps []string

		// Add explicit dependencies
		for _, dep := range task.DependsOn {
			deps = append(deps, dep.TaskID)
		}

		// Add implicit dependencies from template references in if_else_task
		if task.IfElseTask != nil {
			// Check inputs field for template references
			if task.IfElseTask.Inputs != "" {
				if implicitDeps := c.extractTaskDependenciesFromTemplate(task.IfElseTask.Inputs); len(implicitDeps) > 0 {
					deps = append(deps, implicitDeps...)
				}
			}
			// Check condition fields for template references
			if task.IfElseTask.Condition != nil {
				if implicitDeps := c.extractTaskDependenciesFromTemplate(task.IfElseTask.Condition.Left); len(implicitDeps) > 0 {
					deps = append(deps, implicitDeps...)
				}
				if implicitDeps := c.extractTaskDependenciesFromTemplate(task.IfElseTask.Condition.Right); len(implicitDeps) > 0 {
					deps = append(deps, implicitDeps...)
				}
			}
		}

		// Remove duplicates
		deps = c.removeDuplicates(deps)
		dependencies[effectiveID] = deps
	}

	// Topological sort using Kahn's algorithm
	inDegree := make(map[string]int)
	for taskID := range taskMap {
		inDegree[taskID] = 0
	}
	for taskID, deps := range dependencies {
		for _, dep := range deps {
			if _, exists := taskMap[dep]; exists {
				inDegree[taskID]++
			}
		}
	}

	var queue []string
	for taskID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, taskID)
		}
	}

	var result []*TaskSpec
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, taskMap[current])

		for taskID, deps := range dependencies {
			for _, dep := range deps {
				if dep == current {
					inDegree[taskID]--
					if inDegree[taskID] == 0 {
						queue = append(queue, taskID)
					}
				}
			}
		}
	}

	if len(result) != len(tasks) {
		return nil, fmt.Errorf("circular dependency detected")
	}

	return result, nil
}

// extractTaskDependenciesFromTemplate extracts task names from template references like {{tasks.task_name.output}}
func (c *YAMLToUniflowConverter) extractTaskDependenciesFromTemplate(template string) []string {
	var deps []string

	// Find all {{tasks.task_name...}} patterns
	if strings.Contains(template, "{{tasks.") {
		start := strings.Index(template, "{{tasks.") + 8 // Skip "{{tasks."
		end := strings.Index(template[start:], ".")
		if end > 0 {
			taskName := template[start : start+end]
			if taskName != "" {
				deps = append(deps, taskName)
			}
		}
	}

	return deps
}

// removeDuplicates removes duplicate strings from a slice
func (c *YAMLToUniflowConverter) removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// TaskExecutionContext represents different execution contexts
type TaskExecutionContext string

const (
	ContextRegular     TaskExecutionContext = "regular"
	ContextConditional TaskExecutionContext = "conditional"
	ContextForEach     TaskExecutionContext = "foreach"
)

// generateTaskExecutionCode generates appropriate task execution code based on task type
func (c *YAMLToUniflowConverter) generateTaskExecutionCode(task *TaskSpec, context TaskExecutionContext) (string, error) {
	// Handle ForEach tasks by looking at inner task types
	if task.ForEachTask != nil && context == ContextForEach {
		if task.ForEachTask.NotebookTask != nil {
			// Create a temporary task with notebook task at top level for code generation
			tempTask := &TaskSpec{
				TaskID:       task.TaskID,
				NotebookTask: task.ForEachTask.NotebookTask,
			}
			return c.generateStarlarkNotebookTaskExecution(tempTask), nil
		} else if task.ForEachTask.RayTask != nil {
			tempTask := &TaskSpec{
				TaskID:  task.TaskID,
				RayTask: task.ForEachTask.RayTask,
			}
			return c.generateStarlarkRayTaskExecution(tempTask)
		} else if task.ForEachTask.SparkTask != nil {
			tempTask := &TaskSpec{
				TaskID:    task.TaskID,
				SparkTask: task.ForEachTask.SparkTask,
			}
			return c.generateStarlarkSparkTaskExecution(tempTask)
		} else if task.ForEachTask.SparkOneTask != nil {
			// For SparkOne, default to RayTask generation for now
			tempTask := &TaskSpec{
				TaskID: task.TaskID,
				RayTask: &RayTaskConfig{
					TaskParameters: task.ForEachTask.SparkOneTask.TaskParameters,
					UserParameters: task.ForEachTask.SparkOneTask.UserParameters,
				},
			}
			return c.generateStarlarkRayTaskExecution(tempTask)
		} else {
			// Fallback to RayTask
			tempTask := &TaskSpec{
				TaskID: task.TaskID,
				RayTask: &RayTaskConfig{
					TaskParameters: map[string]interface{}{},
					UserParameters: map[string]interface{}{},
				},
			}
			return c.generateStarlarkRayTaskExecution(tempTask)
		}
	}

	// Determine task type and generate appropriate execution code for regular tasks
	if task.NotebookTask != nil {
		return c.generateStarlarkNotebookTaskExecution(task), nil
	} else if task.RayTask != nil {
		return c.generateStarlarkRayTaskExecution(task)
	} else if task.SparkTask != nil {
		return c.generateStarlarkSparkTaskExecution(task)
	} else {
		// Default to RayTask
		return c.generateStarlarkRayTaskExecution(task)
	}
}

// generateStarlarkTaskExecution generates Starlark code for a single task
func (c *YAMLToUniflowConverter) generateStarlarkTaskExecution(task *TaskSpec) (string, error) {
	var taskCode strings.Builder

	// Comment with task info
	taskCode.WriteString(fmt.Sprintf("# Task: %s\n", task.GetEffectiveTaskID()))

	// Check for conditional execution based on dependencies
	hasConditions := c.hasConditionalDependencies(task)

	if hasConditions {
		return c.generateStarlarkConditionalExecution(task)
	}

	// Handle ForEach/expand pattern
	if task.ForEachTask != nil {
		return c.generateStarlarkForEachExecution(task)
	}

	// Handle if_else_task pattern
	if task.IfElseTask != nil {
		return c.generateStarlarkIfElseExecution(task)
	}

	// Regular single task execution
	executionCode, err := c.generateTaskExecutionCode(task, ContextRegular)
	if err != nil {
		return "", err
	}
	taskCode.WriteString(executionCode)

	return taskCode.String(), nil
}

// hasConditionalDependencies checks if task has conditional dependencies
func (c *YAMLToUniflowConverter) hasConditionalDependencies(task *TaskSpec) bool {
	for _, dep := range task.DependsOn {
		if dep.Outcome != "" {
			return true
		}
	}
	return false
}

// generateStarlarkConditionalExecution generates conditional task execution
func (c *YAMLToUniflowConverter) generateStarlarkConditionalExecution(task *TaskSpec) (string, error) {
	var taskCode strings.Builder
	safeName := c.safeStarlarkName(task.GetEffectiveTaskID())

	taskCode.WriteString(fmt.Sprintf("# Task: %s (conditional)\n", task.GetEffectiveTaskID()))

	// Build condition from dependencies
	var conditions []string
	for _, dep := range task.DependsOn {
		if dep.Outcome != "" {
			depSafeName := c.safeStarlarkName(dep.TaskID)
			// For if_else_task outcomes, always check the outcome field
			// This handles both string outcomes like "true"/"false" and custom outcomes
			conditions = append(conditions, fmt.Sprintf("%s_result.get(\"outcome\") == \"%s\"", depSafeName, dep.Outcome))
		}
	}

	if len(conditions) > 0 {
		condition := strings.Join(conditions, " and ")
		taskCode.WriteString(fmt.Sprintf("if %s:\n", condition))

		// Generate task execution inside the conditional
		var innerTaskCode string
		if task.ForEachTask != nil {
			innerCode, err := c.generateStarlarkForEachExecution(task)
			if err != nil {
				return "", err
			}
			innerTaskCode = innerCode
		} else {
			var err error
			innerTaskCode, err = c.generateTaskExecutionCode(task, ContextConditional)
			if err != nil {
				return "", err
			}
		}

		// Indent task lines
		lines := strings.Split(strings.TrimSpace(innerTaskCode), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				taskCode.WriteString(fmt.Sprintf("    %s\n", line))
			} else {
				taskCode.WriteString("\n")
			}
		}

		// Add else clause to set result to None
		taskCode.WriteString("else:\n")
		taskCode.WriteString(fmt.Sprintf("    %s_result = None\n\n", safeName))
	}

	return taskCode.String(), nil
}

// generateStarlarkNotebookTaskExecution generates notebook task execution using ray_task
func (c *YAMLToUniflowConverter) generateStarlarkNotebookTaskExecution(task *TaskSpec) string {
	safeName := c.safeStarlarkName(task.GetEffectiveTaskID())

	var taskCode strings.Builder

	// Generate user parameter variable assignments
	var userVarAssignments []string
	var userVarNames []string

	if task.NotebookTask != nil {
		// Add notebook_path as a parameter
		if task.NotebookTask.NotebookPath != "" {
			userVarAssignments = append(userVarAssignments, fmt.Sprintf("notebook_path = \"%s\"", task.NotebookTask.NotebookPath))
			userVarNames = append(userVarNames, "notebook_path")
		}

		// Add user parameters
		if task.NotebookTask.UserParameters != nil {
			for key, value := range task.NotebookTask.UserParameters {
				if strValue, ok := value.(string); ok && strings.Contains(strValue, "{{tasks.") {
					// Handle template expressions
					refVar := c.convertStarlarkTemplateReference(strValue)
					userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, refVar))
					userVarNames = append(userVarNames, key)
				} else {
					// Static values
					userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, c.formatStarlarkValue(value)))
					userVarNames = append(userVarNames, key)
				}
			}
		}
	}

	// Add user parameter variable assignments
	for _, assignment := range userVarAssignments {
		taskCode.WriteString(fmt.Sprintf("%s\n", assignment))
	}

	// Use ray_task with hardcoded notebook executor path
	taskCode.WriteString(fmt.Sprintf("%s_result = __ray_task__(\n", safeName))
	taskCode.WriteString("    \"examples.notebook_workflow.executor.notebook_executor\",\n")

	// Add task parameters (head_cpu, head_memory, etc.) from YAML spec
	rayTaskParams := []string{"head_cpu", "head_memory", "worker_cpu", "worker_memory", "worker_instances",
		"worker_min_instances", "worker_max_instances", "cache_enabled", "cache_version"}

	for _, paramName := range rayTaskParams {
		if task.NotebookTask != nil && task.NotebookTask.TaskParameters != nil {
			if value, exists := task.NotebookTask.TaskParameters[paramName]; exists {
				if paramName == "cache_enabled" {
					// Handle boolean values
					if strValue := fmt.Sprintf("%v", value); strValue == "false" {
						taskCode.WriteString("    cache_enabled=False,\n")
					} else {
						taskCode.WriteString("    cache_enabled=True,\n")
					}
				} else if paramName == "cache_version" && (value == nil || fmt.Sprintf("%v", value) == "null") {
					taskCode.WriteString("    cache_version=None,\n")
				} else if strValue, ok := value.(string); ok {
					// String values need quotes
					taskCode.WriteString(fmt.Sprintf("    %s=\"%s\",\n", paramName, strValue))
				} else {
					// Numeric values
					taskCode.WriteString(fmt.Sprintf("    %s=%v,\n", paramName, value))
				}
			}
		}
	}

	// Set defaults if not specified in YAML
	if task.NotebookTask == nil || task.NotebookTask.TaskParameters == nil ||
		task.NotebookTask.TaskParameters["cache_enabled"] == nil {
		taskCode.WriteString("    cache_enabled=True,\n")
	}
	if task.NotebookTask == nil || task.NotebookTask.TaskParameters == nil ||
		task.NotebookTask.TaskParameters["cache_version"] == nil {
		taskCode.WriteString("    cache_version=None,\n")
	}

	// Close the __ray_task__ call and add user parameters as keyword arguments
	// Use keyword arguments to eliminate order dependency
	if len(userVarNames) > 0 {
		var keywordArgs []string
		for _, varName := range userVarNames {
			keywordArgs = append(keywordArgs, fmt.Sprintf("%s=%s", varName, varName))
		}
		taskCode.WriteString(fmt.Sprintf(")(%s)\n\n", strings.Join(keywordArgs, ", ")))
	} else {
		taskCode.WriteString(")()\n\n")
	}

	return taskCode.String()
}

// generateStarlarkRayTaskExecution generates Ray task execution
func (c *YAMLToUniflowConverter) generateStarlarkRayTaskExecution(task *TaskSpec) (string, error) {
	safeName := c.safeStarlarkName(task.GetEffectiveTaskID())

	// Extract function from task_parameters - REQUIRED
	fn, exists := task.RayTask.TaskParameters["function"]
	if !exists {
		return "", fmt.Errorf("ray_task '%s' must specify 'function' in task_parameters", task.GetEffectiveTaskID())
	}
	functionName := fmt.Sprintf("%v", fn)

	// Generate user parameter variable assignments
	var userVarAssignments []string
	var userVarNames []string

	for key, value := range task.RayTask.UserParameters {
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "{{tasks.") {
			// Handle template expressions like "{{tasks.some_task.output.field}}"
			// Convert to variable reference: some_task_result.field or some_task_result
			refVar := c.convertStarlarkTemplateReference(strValue)
			userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, refVar))
			userVarNames = append(userVarNames, key)
		} else {
			// Static values - assign to variables
			userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, c.formatStarlarkValue(value)))
			userVarNames = append(userVarNames, key)
		}
	}

	var taskCode strings.Builder

	// Add user parameter variable assignments
	for _, assignment := range userVarAssignments {
		taskCode.WriteString(fmt.Sprintf("%s\n", assignment))
	}

	// Generic result assignment
	resultAssignment := fmt.Sprintf("%s_result = ", safeName)

	// Start the __ray_task__ call
	taskCode.WriteString(fmt.Sprintf("%s__ray_task__(\n", resultAssignment))
	taskCode.WriteString(fmt.Sprintf("    \"%s\",\n", functionName))

	// Add task parameters (head_cpu, head_memory, etc.)
	rayTaskParams := []string{"head_cpu", "head_memory", "worker_cpu", "worker_memory", "worker_instances",
		"worker_min_instances", "worker_max_instances", "cache_enabled", "cache_version"}

	for _, paramName := range rayTaskParams {
		if value, exists := task.RayTask.TaskParameters[paramName]; exists {
			if paramName == "cache_enabled" {
				// Handle boolean values
				if strValue := fmt.Sprintf("%v", value); strValue == "false" {
					taskCode.WriteString("    cache_enabled=False,\n")
				} else {
					taskCode.WriteString("    cache_enabled=True,\n")
				}
			} else if paramName == "cache_version" && (value == nil || fmt.Sprintf("%v", value) == "null") {
				taskCode.WriteString("    cache_version=None,\n")
			} else if strValue, ok := value.(string); ok {
				// String values need quotes
				taskCode.WriteString(fmt.Sprintf("    %s=\"%s\",\n", paramName, strValue))
			} else {
				// Numeric values
				taskCode.WriteString(fmt.Sprintf("    %s=%v,\n", paramName, value))
			}
		}
	}

	// Close the __ray_task__ call and add user parameters as keyword arguments
	if len(userVarNames) > 0 {
		var keywordArgs []string
		for _, varName := range userVarNames {
			keywordArgs = append(keywordArgs, fmt.Sprintf("%s=%s", varName, varName))
		}
		taskCode.WriteString(fmt.Sprintf(")(%s)\n\n", strings.Join(keywordArgs, ", ")))
	} else {
		taskCode.WriteString(")()\n\n")
	}

	return taskCode.String(), nil
}

// generateStarlarkSparkTaskExecution generates Spark task execution
func (c *YAMLToUniflowConverter) generateStarlarkSparkTaskExecution(task *TaskSpec) (string, error) {
	safeName := c.safeStarlarkName(task.TaskID)

	// Extract function from task_parameters - REQUIRED
	fn, exists := task.SparkTask.TaskParameters["function"]
	if !exists {
		return "", fmt.Errorf("spark_task '%s' must specify 'function' in task_parameters", task.TaskID)
	}
	functionName := fmt.Sprintf("%v", fn)

	// Generate user parameter variable assignments
	var userVarAssignments []string
	var userVarNames []string

	for key, value := range task.SparkTask.UserParameters {
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "{{tasks.") {
			// Handle template expressions
			refVar := c.convertStarlarkTemplateReference(strValue)
			userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, refVar))
			userVarNames = append(userVarNames, key)
		} else {
			// Static values - assign to variables
			userVarAssignments = append(userVarAssignments, fmt.Sprintf("%s = %s", key, c.formatStarlarkValue(value)))
			userVarNames = append(userVarNames, key)
		}
	}

	var taskCode strings.Builder

	// Add user parameter variable assignments
	for _, assignment := range userVarAssignments {
		taskCode.WriteString(fmt.Sprintf("%s\n", assignment))
	}

	// Generic result assignment
	resultAssignment := fmt.Sprintf("%s_result = ", safeName)

	// Start the __spark_task__ call
	taskCode.WriteString(fmt.Sprintf("%s__spark_task__(\n", resultAssignment))
	taskCode.WriteString(fmt.Sprintf("    \"%s\",\n", functionName))

	// Add task parameters (driver_cpu, driver_memory, etc.)
	sparkTaskParams := []string{"driver_cpu", "driver_memory", "executor_cpu", "executor_instances",
		"executor_memory", "cache_enabled", "cache_version"}

	for _, paramName := range sparkTaskParams {
		if value, exists := task.SparkTask.TaskParameters[paramName]; exists {
			if paramName == "cache_enabled" {
				// Handle boolean values
				if strValue := fmt.Sprintf("%v", value); strValue == "false" {
					taskCode.WriteString("    cache_enabled=False,\n")
				} else {
					taskCode.WriteString("    cache_enabled=True,\n")
				}
			} else if paramName == "cache_version" && (value == nil || fmt.Sprintf("%v", value) == "null") {
				taskCode.WriteString("    cache_version=None,\n")
			} else if strValue, ok := value.(string); ok {
				// String values need quotes
				taskCode.WriteString(fmt.Sprintf("    %s=\"%s\",\n", paramName, strValue))
			} else {
				// Numeric values
				taskCode.WriteString(fmt.Sprintf("    %s=%v,\n", paramName, value))
			}
		}
	}

	// Close the __spark_task__ call and add user parameters as keyword arguments
	if len(userVarNames) > 0 {
		var keywordArgs []string
		for _, varName := range userVarNames {
			keywordArgs = append(keywordArgs, fmt.Sprintf("%s=%s", varName, varName))
		}
		taskCode.WriteString(fmt.Sprintf(")(%s)\n\n", strings.Join(keywordArgs, ", ")))
	} else {
		taskCode.WriteString(")()\n\n")
	}

	return taskCode.String(), nil
}

// generateStarlarkForEachExecution generates foreach/expand pattern execution
func (c *YAMLToUniflowConverter) generateStarlarkForEachExecution(task *TaskSpec) (string, error) {
	safeName := c.safeStarlarkName(task.GetEffectiveTaskID())

	var taskCode strings.Builder
	taskCode.WriteString(fmt.Sprintf("# Task: %s (foreach)\n", task.GetEffectiveTaskID()))

	iterSource := c.convertForEachInputsForStarlark(task.ForEachTask.Inputs)

	taskCode.WriteString(fmt.Sprintf("%s_results = []\n", safeName))
	taskCode.WriteString(fmt.Sprintf("for item_value in %s:\n", iterSource))

	// Generate the appropriate task execution code based on task type
	executionCode, err := c.generateTaskExecutionCode(task, ContextForEach)
	if err != nil {
		return "", err
	}

	// Modify the generated code to use 'iteration_result' instead of the task name result
	// Replace the task result assignment with iteration_result assignment
	modifiedCode := strings.ReplaceAll(executionCode, fmt.Sprintf("%s_result = ", safeName), "iteration_result = ")

	// Remove any comments from the execution code since we already have a comment for the ForEach task
	lines := strings.Split(modifiedCode, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			cleanedLines = append(cleanedLines, line)
		}
	}
	modifiedCode = strings.Join(cleanedLines, "\n")

	// Replace {{item}} with item_value for ForEach context
	modifiedCode = strings.ReplaceAll(modifiedCode, `"{{item}}"`, "item_value")
	modifiedCode = strings.ReplaceAll(modifiedCode, "{{item}}", "item_value")

	// Indent the execution code for the for loop
	lines = strings.Split(strings.TrimSpace(modifiedCode), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			taskCode.WriteString(fmt.Sprintf("    %s\n", line))
		}
	}

	taskCode.WriteString(fmt.Sprintf("    %s_results.append(iteration_result)\n", safeName))
	taskCode.WriteString(fmt.Sprintf("%s_result = %s_results\n\n", safeName, safeName))

	return taskCode.String(), nil
}

// generateStarlarkIfElseExecution generates Starlark code for if_else_task
func (c *YAMLToUniflowConverter) generateStarlarkIfElseExecution(task *TaskSpec) (string, error) {
	safeName := c.safeStarlarkName(task.GetEffectiveTaskID())

	var taskCode strings.Builder
	taskCode.WriteString(fmt.Sprintf("# Task: %s (if_else)\n", task.GetEffectiveTaskID()))

	if task.IfElseTask == nil {
		return "", fmt.Errorf("if_else_task configuration is required for task %s", task.TaskID)
	}

	if task.IfElseTask.Condition == nil {
		return "", fmt.Errorf("condition is required for if_else_task %s", task.TaskID)
	}

	// Extract the inputs (usually a task reference)
	inputs := task.IfElseTask.Inputs
	if inputs == "" {
		return "", fmt.Errorf("inputs is required for if_else_task %s", task.TaskID)
	}

	// Convert template reference to Starlark variable reference
	inputsRef := c.convertStarlarkTemplateReference(inputs)

	// Generate condition evaluation code
	condition := task.IfElseTask.Condition
	leftRef := c.convertStarlarkTemplateReference(condition.Left)
	rightValue := condition.Right // Assume right side is a literal value

	// Build the condition expression based on operator
	var conditionExpr string
	switch condition.Op {
	case "EQUAL_TO", "EQ":
		conditionExpr = fmt.Sprintf("%s == \"%s\"", leftRef, rightValue)
	case "NOT_EQUAL_TO", "NE":
		conditionExpr = fmt.Sprintf("%s != \"%s\"", leftRef, rightValue)
	case "GREATER_THAN", "GT":
		conditionExpr = fmt.Sprintf("%s > %s", leftRef, rightValue)
	case "LESS_THAN", "LT":
		conditionExpr = fmt.Sprintf("%s < %s", leftRef, rightValue)
	case "GREATER_EQUAL", "GE":
		conditionExpr = fmt.Sprintf("%s >= %s", leftRef, rightValue)
	case "LESS_EQUAL", "LE":
		conditionExpr = fmt.Sprintf("%s <= %s", leftRef, rightValue)
	default:
		return "", fmt.Errorf("unsupported condition operator '%s' for task %s", condition.Op, task.TaskID)
	}

	// Generate the if_else logic
	taskCode.WriteString(fmt.Sprintf("# Evaluate condition: %s\n", conditionExpr))
	taskCode.WriteString(fmt.Sprintf("if %s:\n", conditionExpr))
	taskCode.WriteString(fmt.Sprintf("    %s_result = {\"outcome\": \"true\", \"inputs\": %s}\n", safeName, inputsRef))
	taskCode.WriteString("else:\n")
	taskCode.WriteString(fmt.Sprintf("    %s_result = {\"outcome\": \"false\", \"inputs\": %s}\n\n", safeName, inputsRef))

	return taskCode.String(), nil
}

// safeStarlarkName converts to valid Starlark identifier
func (c *YAMLToUniflowConverter) safeStarlarkName(name string) string {
	safe := strings.ReplaceAll(name, "-", "_")
	safe = strings.ReplaceAll(safe, ".", "_")
	safe = strings.ReplaceAll(safe, " ", "_")
	return safe
}

// formatStarlarkValue formats a value for Starlark code
func (c *YAMLToUniflowConverter) formatStarlarkValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		// Check for template references
		if strings.Contains(v, "{{") && strings.Contains(v, "}}") {
			return c.convertStarlarkTemplateReference(v)
		}
		return fmt.Sprintf("\"%s\"", v)
	case int:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%f", v)
	case bool:
		if v {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprintf("\"%v\"", v)
	}
}

// convertStarlarkTemplateReference converts template references for Starlark
func (c *YAMLToUniflowConverter) convertStarlarkTemplateReference(ref string) string {
	// Convert template references like {{tasks.taskname.output.field}} to Starlark
	if strings.Contains(ref, "{{tasks.") {
		// Remove {{ and }}
		inner := strings.Trim(ref, "{}")
		inner = strings.TrimSpace(inner)

		if strings.HasPrefix(inner, "tasks.") {
			parts := strings.Split(inner, ".")
			if len(parts) >= 2 {
				taskName := parts[1]
				safeName := c.safeStarlarkName(taskName)

				// Check if this is a notebook task (returns tuple)
				isNotebookTask := c.taskTypes[taskName] == "notebook"

				if len(parts) > 2 {
					// Handle nested access like tasks.validation.output.status or tasks.validation.task_values.key
					accessPath := strings.Join(parts[2:], ".")

					if accessPath == "output" {
						if isNotebookTask {
							return fmt.Sprintf("%s_result[0]", safeName)  // First element of tuple (exit_value)
						}
						return fmt.Sprintf("%s_result", safeName)
					}

					if accessPath == "task_values" {
						if isNotebookTask {
							return fmt.Sprintf("%s_result[1]", safeName)  // Second element of tuple (task_values)
						}
						return fmt.Sprintf("%s_result", safeName)  // Other tasks don't have separate task_values
					}

					// Convert output.status to result.get("status") or result[0].get("status") for notebook tasks
					if strings.HasPrefix(accessPath, "output.") {
						field := strings.TrimPrefix(accessPath, "output.")
						if isNotebookTask {
							return fmt.Sprintf("%s_result[0].get(\"%s\")", safeName, field)
						}
						return fmt.Sprintf("%s_result.get(\"%s\")", safeName, field)
					}

					// Convert task_values.key to result[1].get("key") for notebook tasks
					if strings.HasPrefix(accessPath, "task_values.") {
						field := strings.TrimPrefix(accessPath, "task_values.")
						if isNotebookTask {
							return fmt.Sprintf("%s_result[1].get(\"%s\")", safeName, field)
						}
						return fmt.Sprintf("%s_result.get(\"%s\")", safeName, field)  // Other tasks don't have separate task_values
					}
				}
				return fmt.Sprintf("%s_result", safeName)
			}
		}
	}

	// Return as quoted string if no template
	return fmt.Sprintf("\"%s\"", ref)
}

// createUniflowTar creates a tar.gz archive with Uniflow files
func (c *YAMLToUniflowConverter) createUniflowTar(uniflowYAML, starlarkCode string, dagSpec *DAGFactorySpec) ([]byte, error) {
	var tarBuffer bytes.Buffer
	gzWriter := gzip.NewWriter(&tarBuffer)
	tarWriter := tar.NewWriter(gzWriter)

	// Add workflow.yaml file
	if err := c.addFileToTar(tarWriter, "workflow.yaml", []byte(uniflowYAML)); err != nil {
		return nil, fmt.Errorf("failed to add workflow.yaml: %w", err)
	}

	// Add workflow.py file
	if err := c.addFileToTar(tarWriter, "workflow.py", []byte(starlarkCode)); err != nil {
		return nil, fmt.Errorf("failed to add workflow.py: %w", err)
	}

	// Add bundled Starlark plugin files
	if err := c.addStarlarkPluginFiles(tarWriter); err != nil {
		return nil, fmt.Errorf("failed to add Starlark plugin files: %w", err)
	}

	// Add meta.json file with workflow metadata
	if err := c.addMetadataFile(tarWriter, dagSpec); err != nil {
		return nil, fmt.Errorf("failed to add meta.json: %w", err)
	}

	// Close writers
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	c.logger.Info("Created Uniflow tar", zap.Int("size_bytes", tarBuffer.Len()), zap.String("workflow", dagSpec.Pipeline.Name))
	return tarBuffer.Bytes(), nil
}

// getConsolidatedTasks returns tasks from either pipeline.tasks or top-level tasks
func (c *YAMLToUniflowConverter) getConsolidatedTasks(dagSpec *DAGFactorySpec) []*TaskSpec {
	// Check if tasks are defined at the top level (new format)
	if len(dagSpec.Tasks) > 0 {
		return dagSpec.Tasks
	}

	// Fallback to pipeline.tasks (old format)
	if dagSpec.Pipeline != nil && len(dagSpec.Pipeline.Tasks) > 0 {
		return dagSpec.Pipeline.Tasks
	}

	// No tasks found
	return []*TaskSpec{}
}

// addFileToTar adds a file to the tar archive
func (c *YAMLToUniflowConverter) addFileToTar(tarWriter *tar.Writer, filename string, content []byte) error {
	header := &tar.Header{
		Name:    filename,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err := tarWriter.Write(content)
	return err
}

// addStarlarkPluginFiles adds Starlark plugin files to the tar archive
func (c *YAMLToUniflowConverter) addStarlarkPluginFiles(tarWriter *tar.Writer) error {
	// Get the directory where this Go file is located
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("failed to get current file path")
	}
	currentDir := filepath.Dir(currentFile)

	// Define the Starlark files to bundle
	starlarkFiles := []string{"commons.star", "ray_task.star", "spark_task.star"}

	for _, filename := range starlarkFiles {
		filePath := filepath.Join(currentDir, filename)
		content, err := ioutil.ReadFile(filePath)
		if err != nil {
			c.logger.Warn("Failed to read Starlark plugin file, skipping",
				zap.String("file", filename),
				zap.String("path", filePath),
				zap.Error(err))
			continue
		}

		if err := c.addFileToTar(tarWriter, filename, content); err != nil {
			return fmt.Errorf("failed to add %s to tar: %w", filename, err)
		}

		c.logger.Debug("Added Starlark plugin file to tar",
			zap.String("file", filename),
			zap.Int("size", len(content)))
	}

	return nil
}

// addMetadataFile adds meta.json file with workflow execution metadata to the tar archive
func (c *YAMLToUniflowConverter) addMetadataFile(tarWriter *tar.Writer, dagSpec *DAGFactorySpec) error {
	// Generate workflow function name using same logic as generateStarlarkWorkflowFunction
	workflowFunctionName := c.safeStarlarkName(dagSpec.Pipeline.Name)

	// Create metadata structure
	metadata := map[string]interface{}{
		"main_file":     "workflow.py",
		"main_function": workflowFunctionName,
	}

	// Convert to JSON
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata to JSON: %w", err)
	}

	// Add to tar
	if err := c.addFileToTar(tarWriter, "meta.json", metaJSON); err != nil {
		return fmt.Errorf("failed to add meta.json to tar: %w", err)
	}

	c.logger.Debug("Added meta.json to tar",
		zap.String("main_file", "workflow.py"),
		zap.String("main_function", workflowFunctionName),
		zap.Int("size", len(metaJSON)))

	return nil
}
