package actors

import (
	"context"
	"encoding/base64"
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