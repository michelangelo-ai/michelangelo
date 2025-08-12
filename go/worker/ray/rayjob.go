package ray

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ListRayJobsResponse represents the response from listing RayJobs.
type ListRayJobsResponse struct {
	// Items is a list of RayJob objects.
	Items []unstructured.Unstructured `json:"items"`
}

// GetRayJobResponse represents the response from retrieving a RayJob.
type GetRayJobResponse struct {
	// Object contains the details of the RayJob.
	Object map[string]interface{} `json:"object"`
}

// CreateRayJobRequest represents the request for creating a new RayJob.
type CreateRayJobRequest struct {
	// RayJob contains the details to patch the RayJob before creating it.
	RayJob `json:",inline"`
}

// CreateRayJobResponse represents the response from creating a RayJob.
type CreateRayJobResponse struct {
	// Object contains the details of the created RayJob.
	Object map[string]interface{} `json:"object"`
}

// Metadata represents metadata for a RayJob.
type Metadata struct {
	Name string `json:"name,omitempty"`
}

// Spec represents the specification of a RayJob.
type Spec struct {
	Pipeline              string         `json:"pipeline,omitempty"`
	ActiveDeadlineSeconds int            `json:"activeDeadlineSeconds,omitempty"`
	Entrypoint            string         `json:"entrypoint,omitempty"`
	RayClusterSpec        RayClusterSpec `json:"rayClusterSpec,omitempty"`
	SubmitterImage        string         `json:"submitterImage,omitempty"`
	CPU                   string         `json:"cpu,omitempty"`
	Memory                string         `json:"memory,omitempty"`
}

// RayJob represents a Ray job resource.
type RayJob struct {
	APIVersion string   `json:"apiVersion,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Metadata   Metadata `json:"metadata,omitempty"`
	Spec       Spec     `json:"spec,omitempty"`
}
