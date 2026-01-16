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