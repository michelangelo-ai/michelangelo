package actors

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	v2 "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestYAMLToUniflowConverter(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Sample DAG Factory YAML
	dagFactoryYAML := `spec_version: 1.0
pipeline:
  type: SCHEDULED
  name: "test-pipeline"
  owner: "test@uber.com"
  workspace: "component:test-workspace"
  schedule:
    crontab: "0 10 * * *"
    max_concurrency: 1
  description: "Test pipeline"
  metadata_tags: ["test", "demo"]

tasks:
  - task_id: extract_data
    notebook_task:
      notebook_path: /Workspace/test/extract.ipynb
      user_parameters:
        ref_date: "{{pipeline.start_time.iso_date}}"
        environment: "test"

  - task_id: validate_data
    depends_on:
      - task_id: extract_data
        outcome: "true"
    notebook_task:
      notebook_path: /Workspace/test/validate.ipynb
      user_parameters:
        data_source: "{{tasks.extract_data.output}}"
`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(dagFactoryYAML))

	// Create BytesValue using gogo protobuf
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any using gogo protobuf
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pipeline",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated")
	}

	t.Logf("Successfully generated tar of %d bytes", len(tarBytes))
}

func TestExtractYAMLContent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	testYAML := "test: yaml\ncontent: here"
	base64Content := base64.StdEncoding.EncodeToString([]byte(testYAML))

	// Create BytesValue using gogo protobuf
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any using gogo protobuf
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Test extraction
	extracted, err := converter.extractYAMLContent(contentAny)
	if err != nil {
		t.Fatalf("Failed to extract YAML content: %v", err)
	}

	if extracted != testYAML {
		t.Fatalf("Extracted content mismatch. Expected: %s, Got: %s", testYAML, extracted)
	}

	t.Logf("Successfully extracted YAML content: %s", extracted)
}

func TestYAMLToUniflowConverterForEach(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test ForEach functionality with Pipeline YML Spec 1.0 format
	dagFactoryYAML := `# DAG Factory For-Each Pattern Test
# Tests the new Pipeline YML Spec 1.0 format with ForEach tasks
spec_version: 1.0
pipeline:
  id: "test-foreach-pipeline-001"
  name: "test-foreach-pipeline"
  created_by: "test@uber.com"
  created_time: 1737504000
  workspace: "component:test-foreach-workspace"
  trigger_settings:
    mode: "SCHEDULED"
    crontab: "0 10 * * *"
    start_date: "2026-01-16"
    end_date: "2027-01-16"
    catchup: "false"
    max_concurrency: 1
  sla: "02:00:00"
  timeout: "04:00:00"
  notifications:
    slack_config:
      channel: "#test-alerts"
      user_oncall: "test-team-oncall"
    splunk_config:
      oncall: "test-team"
  description: "Test pipeline for ForEach functionality"
  tags: ["test", "foreach", "dag-factory"]
  pipeline_artifacts:
    location: "s3://test-foreach-artifacts"

  tasks:
    # Setup task to generate configuration parameters (replaces hardcoded arrays)
    - task_id: setup_configurations
      ray_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "test.config.generate_test_configs"
          head_cpu: "1"
          head_memory: "2Gi"
          worker_instances: "1"
        user_parameters:
          config_type: "test_variants"
          options: "[1, 2, 3]"

    # ForEach task with string template reference (correct spec format)
    - task_id: process_variants
      depends_on:
        - task_id: setup_configurations
      for_each_task:
        inputs: "{{tasks.setup_configurations.output}}"
        concurrency: 3
      ray_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "test.processing.process_item"
          head_cpu: "1"
          head_memory: "2Gi"
          worker_instances: "1"
        user_parameters:
          item_data: "{{item.data}}"
          item_config: "{{item.config}}"

    # Another ForEach task chaining the previous output
    - task_id: validate_results
      depends_on:
        - task_id: process_variants
      for_each_task:
        inputs: "{{tasks.process_variants.output}}"
        concurrency: 2
      ray_task:
        task_parameters:
          cache_enabled: "false"
          cache_version: null
          function: "test.validation.validate_item"
          head_cpu: "1"
          head_memory: "1Gi"
          worker_instances: "1"
        user_parameters:
          validation_data: "{{item.result}}"
          validation_type: "{{item.type}}"

    # Final aggregation task (regular task, not ForEach)
    - task_id: aggregate_results
      depends_on:
        - task_id: validate_results
      ray_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "test.aggregation.aggregate_all"
          head_cpu: "2"
          head_memory: "4Gi"
          worker_instances: "1"
        user_parameters:
          all_results: "{{tasks.validate_results.output}}"
          summary_type: "final"
`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(dagFactoryYAML))

	// Create BytesValue using gogo protobuf
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any using gogo protobuf
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-foreach-pipeline",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("ForEach conversion failed: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for ForEach test")
	}

	t.Logf("Successfully generated ForEach tar of %d bytes", len(tarBytes))
}

func TestForEachInputsParsing(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test that string template references parse correctly
	testCases := []struct {
		name         string
		inputYAML    string
		expectError  bool
		description  string
	}{
		{
			name: "ValidStringTemplateReference",
			inputYAML: `spec_version: 1.0
pipeline:
  id: "test-001"
  name: "test-pipeline"
  created_by: "test@uber.com"
  created_time: 1737504000
  workspace: "component:test"
  tasks:
    - task_id: setup_task
      ray_task:
        task_parameters:
          function: "test.setup"
        user_parameters:
          config_type: "test"
    - task_id: test_foreach
      depends_on:
        - task_id: setup_task
      for_each_task:
        inputs: "{{tasks.setup_task.output}}"
        concurrency: 2
      ray_task:
        task_parameters:
          function: "test.func"
        user_parameters:
          data: "{{item.data}}"`,
			expectError: false,
			description: "ForEach with string template reference should parse successfully",
		},
		{
			name: "ValidChainedTaskOutputs",
			inputYAML: `spec_version: 1.0
pipeline:
  id: "test-002"
  name: "test-pipeline"
  created_by: "test@uber.com"
  created_time: 1737504000
  workspace: "component:test"
  tasks:
    - task_id: config_task
      ray_task:
        task_parameters:
          function: "test.config"
    - task_id: process_task
      depends_on:
        - task_id: config_task
      for_each_task:
        inputs: "{{tasks.config_task.output}}"
        concurrency: 3
      ray_task:
        task_parameters:
          function: "test.process"
        user_parameters:
          item_data: "{{item.value}}"
    - task_id: validate_task
      depends_on:
        - task_id: process_task
      for_each_task:
        inputs: "{{tasks.process_task.output}}"
        concurrency: 2
      ray_task:
        task_parameters:
          function: "test.validate"
        user_parameters:
          validation_data: "{{item.result}}"`,
			expectError: false,
			description: "Chained ForEach tasks with task output references should parse successfully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encode as base64
			base64Content := base64.StdEncoding.EncodeToString([]byte(tc.inputYAML))

			// Create BytesValue
			bytesValue := &types.BytesValue{
				Value: []byte(base64Content),
			}

			// Wrap in Any
			contentAny, err := types.MarshalAny(bytesValue)
			if err != nil {
				t.Fatalf("Failed to create Any from BytesValue: %v", err)
			}

			// Create mock pipeline
			pipeline := &v2.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.name,
					Namespace: "test-ns",
				},
				Spec: v2.PipelineSpec{
					Manifest: &v2.PipelineManifest{
						Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
						Content: contentAny,
					},
				},
			}

			// Test the conversion
			ctx := context.Background()
			_, err = converter.ConvertYAMLToUniflowTar(ctx, pipeline)

			if tc.expectError && err == nil {
				t.Fatalf("Expected error for %s, but conversion succeeded", tc.description)
			}
			if !tc.expectError && err != nil {
				t.Fatalf("Expected success for %s, but got error: %v", tc.description, err)
			}

			t.Logf("Test case '%s' completed as expected: %s", tc.name, tc.description)
		})
	}
}

func TestSimpleWorkflowPipeline(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test the complete simple workflow with Spark tasks, ForEach, and conditional execution
	simpleWorkflowYAML := `# Simple Workflow Pipeline - DAG Factory Format
# Demonstrates load_data -> preprocess (ForEach) -> train -> eval (conditional)
spec_version: 1.0
pipeline:
  id: "simple-workflow-demo-001"
  name: "simple-workflow-demo"
  created_by: "ml-team@uber.com"
  created_time: 1737504000
  workspace: "component:simple-workflow-workspace"
  trigger_settings:
    mode: "MANUAL"
    catchup: "false"
    max_concurrency: 1
  sla: "01:00:00"
  timeout: "02:00:00"
  notifications:
    slack_config:
      channel: "#simple-workflow-alerts"
      user_oncall: "ml-team-oncall"
  description: "Simple ML workflow for testing DAG Factory to Uniflow conversion"
  tags: ["simple", "workflow", "demo", "dag-factory", "test"]
  pipeline_artifacts:
    location: "s3://simple-workflow-artifacts"

  tasks:
    # Task 1: Load data - Spark task that loads and partitions data
    - task_id: load_data
      spark_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "examples.simple_workflow.simple_workflow.load_data"
          driver_cpu: "2"
          executor_cpu: "1"
        user_parameters:
          data_source: "demo_data"
          num_partitions: 3

    # Task 2: Preprocess partitions - ForEach Spark task processing each partition
    - task_id: preprocess_partitions
      depends_on:
        - task_id: load_data
      for_each_task:
        inputs: "{{tasks.load_data.output}}"
        concurrency: 3
      spark_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "examples.simple_workflow.simple_workflow.preprocess"
          driver_cpu: "1"
          executor_cpu: "1"
        user_parameters:
          partition_data: "{{item}}"
          normalize: true
          remove_nulls: true

    # Task 3: Train model - Ray task that trains on all processed partitions
    - task_id: train_model
      depends_on:
        - task_id: preprocess_partitions
      ray_task:
        task_parameters:
          cache_enabled: "true"
          cache_version: "v1"
          function: "examples.simple_workflow.simple_workflow.train"
          head_cpu: "4"
          head_memory: "8Gi"
          worker_cpu: "2"
          worker_memory: "4Gi"
          worker_instances: "2"
        user_parameters:
          processed_partitions: "{{tasks.preprocess_partitions.output}}"
          model_type: "simple_classifier"
          epochs: 10

    # Task 4: Evaluate model - Conditional Ray task that evaluates if training succeeded
    - task_id: evaluate_model
      depends_on:
        - task_id: train_model
          outcome: "success"
      ray_task:
        task_parameters:
          cache_enabled: "false"
          cache_version: null
          function: "examples.simple_workflow.simple_workflow.eval_model"
          head_cpu: "2"
          head_memory: "4Gi"
          worker_cpu: "1"
          worker_memory: "2Gi"
          worker_instances: "1"
        user_parameters:
          training_result: "{{tasks.train_model.output}}"
          eval_dataset_size: 500`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(simpleWorkflowYAML))

	// Create BytesValue using gogo protobuf
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any using gogo protobuf
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-workflow-demo",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Simple workflow conversion failed: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for simple workflow test")
	}

	t.Logf("Successfully generated simple workflow tar of %d bytes", len(tarBytes))

	// Additional verification: ensure we can extract YAML content properly
	yamlContent, err := converter.extractYAMLContent(contentAny)
	if err != nil {
		t.Fatalf("Failed to extract YAML content: %v", err)
	}

	// Verify the YAML contains our expected workflow structure
	if !strings.Contains(yamlContent, "simple-workflow-demo") {
		t.Fatalf("YAML content doesn't contain pipeline name")
	}
	if !strings.Contains(yamlContent, "for_each_task") {
		t.Fatalf("YAML content doesn't contain ForEach task")
	}
	if !strings.Contains(yamlContent, "spark_task") {
		t.Fatalf("YAML content doesn't contain Spark task")
	}
	if !strings.Contains(yamlContent, "ray_task") {
		t.Fatalf("YAML content doesn't contain Ray task")
	}
	if !strings.Contains(yamlContent, "outcome: \"success\"") {
		t.Fatalf("YAML content doesn't contain conditional execution")
	}

	t.Logf("Simple workflow YAML validation passed")
}

func TestNotebookTaskConversion(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test notebook task with proper resource parameters and dependencies
	notebookWorkflowYAML := `spec_version: "1.0"
pipeline:
  name: "conditional_notebook_workflow"
  created_by: "claude@anthropic.com"
  created_time: 1737504000
  workspace: "component:notebook-workflows"
  description: "Conditional workflow demonstrating exit_value vs task_values usage"
  tags: ["notebook", "conditional", "uniflow"]

  tasks:
    # STEP 1: Data Validation notebook task
    - task_id: "data_validation"
      notebook_task:
        task_parameters:
          head_cpu: 2
          head_memory: "4Gi"
          worker_cpu: 1
          worker_memory: "2Gi"
          worker_instances: 1
          cache_enabled: true
          timeout: "00:30:00"
          kernel_name: "python3"
        user_parameters:
          notebook_path: "examples/notebook_workflow/data_validation.ipynb"
          data_size: "100"
          seed: "42"

    # STEP 2: Advanced Analysis (depends on validation)
    - task_id: "advanced_analysis"
      depends_on:
        - task_id: "data_validation"
      notebook_task:
        task_parameters:
          head_cpu: 4
          head_memory: "8Gi"
          worker_cpu: 2
          worker_memory: "4Gi"
          worker_instances: 2
          cache_enabled: true
          timeout: "00:45:00"
          kernel_name: "python3"
        user_parameters:
          notebook_path: "examples/notebook_workflow/advanced_analysis.ipynb"
          parameters: "{{tasks.data_validation.task_values}}"

    # STEP 3: Basic Analysis (alternative path)
    - task_id: "basic_analysis"
      depends_on:
        - task_id: "data_validation"
      notebook_task:
        task_parameters:
          head_cpu: 1
          head_memory: "2Gi"
          cache_enabled: true
          timeout: "00:30:00"
          kernel_name: "python3"
        user_parameters:
          notebook_path: "examples/notebook_workflow/basic_analysis.ipynb"
          parameters: "{{tasks.data_validation.task_values}}"

parameters:
  data_size: 100
  seed: 42`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(notebookWorkflowYAML))

	// Create BytesValue using gogo protobuf
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any using gogo protobuf
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "conditional-notebook-workflow",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Notebook task conversion failed: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for notebook task test")
	}

	t.Logf("Successfully generated notebook workflow tar of %d bytes", len(tarBytes))
}

func TestNotebookTaskVariousConfigurations(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	testCases := []struct {
		name        string
		inputYAML   string
		description string
	}{
		{
			name: "MinimalNotebookTask",
			inputYAML: `spec_version: "1.0"
pipeline:
  name: "test-notebook"
  created_by: "test@example.com"
  created_time: 1737504000
  workspace: "component:test"

  tasks:
    - task_id: "test_task"
      notebook_task:
        task_parameters:
          head_cpu: 2
          head_memory: "4Gi"
        user_parameters:
          data_size: "100"`,
			description: "Notebook task with minimal configuration should convert successfully",
		},
		{
			name: "NotebookTaskWithoutTaskParams",
			inputYAML: `spec_version: "1.0"
pipeline:
  name: "test-notebook"
  created_by: "test@example.com"
  created_time: 1737504000
  workspace: "component:test"

  tasks:
    - task_id: "test_task"
      notebook_task:
        user_parameters:
          notebook_path: "examples/test.ipynb"
          data_size: "100"`,
			description: "Notebook task without task_parameters should use defaults",
		},
		{
			name: "FullNotebookTask",
			inputYAML: `spec_version: "1.0"
pipeline:
  name: "test-notebook"
  created_by: "test@example.com"
  created_time: 1737504000
  workspace: "component:test"

  tasks:
    - task_id: "test_task"
      notebook_task:
        task_parameters:
          head_cpu: 2
          head_memory: "4Gi"
          worker_cpu: 1
          worker_memory: "2Gi"
          worker_instances: 1
          cache_enabled: true
        user_parameters:
          notebook_path: "examples/test.ipynb"
          data_size: "100"`,
			description: "Full notebook task configuration should succeed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encode as base64
			base64Content := base64.StdEncoding.EncodeToString([]byte(tc.inputYAML))

			// Create BytesValue
			bytesValue := &types.BytesValue{
				Value: []byte(base64Content),
			}

			// Wrap in Any
			contentAny, err := types.MarshalAny(bytesValue)
			if err != nil {
				t.Fatalf("Failed to create Any from BytesValue: %v", err)
			}

			// Create mock pipeline
			pipeline := &v2.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.name,
					Namespace: "test-ns",
				},
				Spec: v2.PipelineSpec{
					Manifest: &v2.PipelineManifest{
						Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
						Content: contentAny,
					},
				},
			}

			// Test the conversion - should always succeed with current implementation
			ctx := context.Background()
			tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)

			if err != nil {
				t.Fatalf("Expected success for %s, but got error: %v", tc.description, err)
			}

			// Verify tar was created with content
			if len(tarBytes) == 0 {
				t.Fatalf("Empty tar generated for %s", tc.description)
			}

			t.Logf("Test case '%s' completed successfully: %s (tar size: %d bytes)", tc.name, tc.description, len(tarBytes))
		})
	}
}

func TestNotebookTaskStarlarkGeneration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test that notebook tasks generate correct Starlark code with ray_task infrastructure
	notebookYAML := `spec_version: "1.0"
pipeline:
  name: "notebook-starlark-test"
  created_by: "test@example.com"
  created_time: 1737504000
  workspace: "component:test"

  tasks:
    - task_id: "notebook_test"
      notebook_task:
        task_parameters:
          head_cpu: 2
          head_memory: "4Gi"
          worker_cpu: 1
          worker_memory: "2Gi"
          worker_instances: 1
          cache_enabled: true
        user_parameters:
          notebook_path: "examples/test.ipynb"
          data_size: "100"`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(notebookYAML))

	// Create BytesValue
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "notebook-starlark-test",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion - this will generate the tar containing Starlark code
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Failed to convert notebook task: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for notebook task test")
	}

	// Extract and verify the generated Starlark code
	starlarkCode := extractStarlarkFromTar(t, tarBytes)

	t.Logf("Generated Starlark code:\n%s", starlarkCode)

	// Verify the generated Starlark code contains expected elements
	expectedElements := []string{
		"load('/ray_task.star', __ray_task__='task')", // Should load ray_task.star for notebook tasks
		"def notebook_starlark_test():",                // Function should match pipeline name
		"__ray_task__(",                               // Should use ray task infrastructure
		"examples.notebook_workflow.executor.notebook_executor", // Should call notebook executor
		"head_cpu=2,",                                 // Should include resource parameters from task_parameters
		"head_memory=\"4Gi\",",
		"worker_cpu=1,",
		"worker_memory=\"2Gi\",",
		"worker_instances=1,",
		"cache_enabled=True,",
		"notebook_path = \"examples/test.ipynb\"",     // Should include user_parameters
		"data_size = \"100\"",
	}

	for _, expected := range expectedElements {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Generated Starlark code missing expected element: %s", expected)
		}
	}

	// Verify the code doesn't contain incorrect elements
	unexpectedElements := []string{
		"load(\"notebook_task.star\"", // Should NOT load notebook_task.star
		"NotebookTask(",              // Should NOT use NotebookTask
	}

	for _, unexpected := range unexpectedElements {
		if strings.Contains(starlarkCode, unexpected) {
			t.Errorf("Generated Starlark code contains unexpected element: %s", unexpected)
		}
	}

	t.Logf("Notebook task Starlark generation test passed")
}

func TestNotebookTaskTemplateExpressions(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test notebook tasks with template expressions from task dependencies
	templateYAML := `spec_version: "1.0"
pipeline:
  name: "notebook-template-test"
  created_by: "test@example.com"
  created_time: 1737504000
  workspace: "component:test"

  tasks:
    - task_id: "data_prep"
      notebook_task:
        task_parameters:
          head_cpu: 1
          head_memory: "2Gi"
        user_parameters:
          notebook_path: "examples/prep.ipynb"

    - task_id: "analysis"
      depends_on:
        - task_id: "data_prep"
      notebook_task:
        task_parameters:
          head_cpu: 2
          head_memory: "4Gi"
        user_parameters:
          notebook_path: "examples/analysis.ipynb"
          input_data: "{{tasks.data_prep.task_values}}"
          parameters: "{{tasks.data_prep.output}}"`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(templateYAML))

	// Create BytesValue
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "notebook-template-test",
			Namespace: "test-ns",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Failed to convert notebook template task: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for notebook template task test")
	}

	// Extract and verify the generated Starlark code
	starlarkCode := extractStarlarkFromTar(t, tarBytes)

	t.Logf("Generated Starlark code:\n%s", starlarkCode)

	// Verify template expressions are converted to proper Starlark variable references
	expectedTemplateConversions := []string{
		"input_data = data_prep_result",  // Template "{{tasks.data_prep.task_values}}" should be converted
		"parameters = data_prep_result",   // Template "{{tasks.data_prep.output}}" should be converted
	}

	for _, expected := range expectedTemplateConversions {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Generated Starlark code missing expected template conversion: %s", expected)
		}
	}

	// Verify dependency structure in generated code
	if !strings.Contains(starlarkCode, "data_prep_result = __ray_task__") {
		t.Errorf("Missing data_prep task call in generated code")
	}
	if !strings.Contains(starlarkCode, "analysis_result = __ray_task__") {
		t.Errorf("Missing analysis task call in generated code")
	}

	// Verify both tasks use the notebook executor
	expectedExecutorCalls := strings.Count(starlarkCode, "examples.notebook_workflow.executor.notebook_executor")
	if expectedExecutorCalls != 2 {
		t.Errorf("Expected 2 notebook executor calls, found %d", expectedExecutorCalls)
	}

	t.Logf("Template expression test passed - all template expressions and dependencies verified")
}

func TestRealWorldNotebookWorkflowFormat(t *testing.T) {
	logger := zaptest.NewLogger(t)
	converter := NewYAMLToUniflowConverter(logger)

	// Test with the EXACT YAML from pipeline.yaml (base64 decoded)
	realWorldYAML := `# Notebook Workflow YAML DAG
# Based on conditional_notebook_workflow.py
# Uses advanced conditional syntax with if_else_task and outcome-based dependencies
spec_version: "1.1"

pipeline:
    id: "conditional-notebook-workflow-001"
    name: "conditional_notebook_workflow"
    created_by: "claude@anthropic.com"
    created_time: 1738368000000  # 2026-01-31 timestamp
    workspace: "component:notebook-workflows"
    trigger_settings:
        mode: "MANUAL"
        start_date: "2026-01-31"
        max_concurrency: 1
    timeout: "01:00:00"
    description: "Conditional workflow demonstrating exit_value vs task_values usage with if_else_task"
    tags: ["notebook", "conditional", "uniflow", "if-else"]

    tasks:
        # STEP 1: Data Validation
        # Returns both exit_value (validation status) and task_values (shared data)
        - task_name: "data_validation"
          notebook_task:
              task_parameters:
                  head_cpu: 2
                  head_memory: "4Gi"
                  worker_cpu: 1
                  worker_memory: "2Gi"
                  worker_instances: 1
                  cache_enabled: true
                  timeout: "00:30:00"
                  kernel_name: "python3"
              user_parameters:
                  notebook_path: "examples/notebook_workflow/data_validation.ipynb"
                  data_size: "{{parameters.data_size}}"
                  seed: "{{parameters.seed}}"

        # STEP 2: Conditional Logic
        # Evaluates validation status to determine which analysis to run
        - task_name: "validation_check"
          if_else_task:
              inputs: "{{tasks.data_validation.output}}"
              condition:
                  op: "EQUAL_TO"
                  left: "{{tasks.data_validation.output.status}}"
                  right: "PASSED"

        # STEP 3: Advanced Analysis (when validation passes)
        # Execute when validation_check evaluates to true
        - task_name: "advanced_analysis"
          depends_on:
              - task_name: "validation_check"
                outcome: "true"
          notebook_task:
              task_parameters:
                  head_cpu: 4
                  head_memory: "8Gi"
                  worker_cpu: 2
                  worker_memory: "4Gi"
                  worker_instances: 2
                  cache_enabled: true
                  timeout: "00:45:00"
                  kernel_name: "python3"
              user_parameters:
                  notebook_path: "examples/notebook_workflow/advanced_analysis.ipynb"
                  # Pass task_values from validation as input parameters
                  parameters: "{{tasks.data_validation.task_values}}"

        # STEP 4: Basic Analysis (when validation fails)
        # Execute when validation_check evaluates to false
        - task_name: "basic_analysis"
          depends_on:
              - task_name: "validation_check"
                outcome: "false"
          notebook_task:
              task_parameters:
                  head_cpu: 1
                  head_memory: "2Gi"
                  cache_enabled: true
                  timeout: "00:30:00"
                  kernel_name: "python3"
              user_parameters:
                  notebook_path: "examples/notebook_workflow/basic_analysis.ipynb"
                  # Pass task_values from validation as input parameters
                  parameters: "{{tasks.data_validation.task_values}}"

# Runtime parameters (equivalent to function parameters)
parameters:
    data_size: 100
    seed: 42`

	// Encode as base64
	base64Content := base64.StdEncoding.EncodeToString([]byte(realWorldYAML))

	// Create BytesValue
	bytesValue := &types.BytesValue{
		Value: []byte(base64Content),
	}

	// Wrap in Any
	contentAny, err := types.MarshalAny(bytesValue)
	if err != nil {
		t.Fatalf("Failed to create Any from BytesValue: %v", err)
	}

	// Create mock pipeline
	pipeline := &v2.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "conditional-notebook-workflow",
			Namespace: "ma-dev-test",
		},
		Spec: v2.PipelineSpec{
			Manifest: &v2.PipelineManifest{
				Type:    v2.PIPELINE_MANIFEST_TYPE_YAML,
				Content: contentAny,
			},
		},
	}

	// Test the conversion
	ctx := context.Background()
	tarBytes, err := converter.ConvertYAMLToUniflowTar(ctx, pipeline)
	if err != nil {
		t.Fatalf("Failed to convert real-world notebook workflow: %v", err)
	}

	// Verify tar was created
	if len(tarBytes) == 0 {
		t.Fatal("Empty tar generated for real-world notebook workflow test")
	}

	// Extract and verify the generated Starlark code
	starlarkCode := extractStarlarkFromTar(t, tarBytes)

	t.Logf("Generated Starlark code for real-world format:\n%s", starlarkCode)

	// Verify notebook task calls are present (only 3 notebook tasks)
	expectedNotebookTaskCalls := []string{
		"data_validation_result = __ray_task__",
	}

	for _, expected := range expectedNotebookTaskCalls {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Missing expected notebook task call: %s", expected)
		}
	}

	// Verify if_else_task conditional logic is generated
	expectedConditionalLogic := []string{
		"# Task: validation_check (if_else)",
		"if data_validation_result.get(\"status\") == \"PASSED\":",
		"validation_check_result = {\"outcome\": \"true\"",
		"validation_check_result = {\"outcome\": \"false\"",
	}

	for _, expected := range expectedConditionalLogic {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Missing expected conditional logic: %s", expected)
		}
	}

	// Verify outcome-based conditional execution
	expectedOutcomeLogic := []string{
		"# Task: advanced_analysis (conditional)",
		"if validation_check_result.get(\"outcome\") == \"true\":",
		"advanced_analysis_result = __ray_task__",
		"# Task: basic_analysis (conditional)",
		"if validation_check_result.get(\"outcome\") == \"false\":",
		"basic_analysis_result = __ray_task__",
	}

	for _, expected := range expectedOutcomeLogic {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Missing expected outcome-based logic: %s", expected)
		}
	}

	// Verify all notebook tasks use the notebook executor (3 calls total)
	expectedExecutorCalls := strings.Count(starlarkCode, "examples.notebook_workflow.executor.notebook_executor")
	if expectedExecutorCalls != 3 {
		t.Errorf("Expected 3 notebook executor calls, found %d", expectedExecutorCalls)
	}

	// Verify template expressions are converted correctly
	expectedTemplateConversions := []string{
		"parameters = data_validation_result",
		"data_validation_result.get(\"status\")",
	}

	for _, expected := range expectedTemplateConversions {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Missing expected template conversion: %s", expected)
		}
	}

	// Verify specific resource parameters
	resourceParams := []string{
		"head_cpu=2,",        // data_validation
		"head_cpu=4,",        // advanced_analysis (conditional)
		"head_cpu=1,",        // basic_analysis (conditional)
		"head_memory=\"8Gi\",", // advanced_analysis
	}

	for _, expected := range resourceParams {
		if !strings.Contains(starlarkCode, expected) {
			t.Errorf("Missing expected resource parameter: %s", expected)
		}
	}

	t.Logf("Real-world notebook workflow with if_else_task test passed - conditional logic and outcome-based dependencies converted correctly")
}

// extractStarlarkFromTar extracts the workflow.py file content from the gzipped tar bytes
func extractStarlarkFromTar(t *testing.T, tarBytes []byte) string {
	// First decompress the gzip
	gzReader, err := gzip.NewReader(bytes.NewReader(tarBytes))
	if err != nil {
		t.Fatalf("Error creating gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Then read the tar
	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error reading tar: %v", err)
		}

		if header.Name == "workflow.py" {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				t.Fatalf("Error reading workflow.py from tar: %v", err)
			}
			return string(content)
		}
	}

	t.Fatal("workflow.py not found in tar")
	return ""
}