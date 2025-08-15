package spark

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ListSparkOneResponse represents the response from listing SparkOne.
type ListSparkOneResponse struct {
	// Items is a list of SparkOne objects.
	Items []unstructured.Unstructured `json:"items"`
}

// GetSparkOneResponse represents the response from retrieving a SparkOne.
type GetSparkOneResponse struct {
	// Object contains the details of the SparkOne.
	Object map[string]interface{} `json:"object"`
}

// CreateSparkOneRequest represents the request for creating a new SparkOne.
type CreateSparkOneRequest struct {
	// SparkOne contains the details to patch the SparkOne before creating it.
	SparkOne `json:",inline"`
}

// CreateSparkOneResponse represents the response from creating a SparkOne.
type CreateSparkOneResponse struct {
	// Object contains the details of the created SparkOne.
	Object map[string]interface{} `json:"object"`
}

// Metadata represents metadata for a SparkOne.
type Metadata struct {
	Name string `json:"name,omitempty"`
}

// Spec represents the specification of a SparkOne.
type Spec struct {
	Pipeline            string `json:"pipeline,omitempty"`
	MainApplicationFile string `json:"mainApplicationFile,omitempty"`
}

// SparkOne represents a SparkOne resource.
type SparkOne struct {
	APIVersion string   `json:"apiVersion,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
	Spec       Spec     `json:"spec,omitempty"`
}
