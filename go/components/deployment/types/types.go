// Package types contains API Schema definitions for deployment types
package types

import (
	api "github.com/michelangelo-ai/michelangelo/proto/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: "michelangelo.api", Version: "v2"}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Deployment{},
		&DeploymentList{},
		&DeploymentEvent{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// TargetType enum for target types
type TargetType string

const (
	TARGET_TYPE_INVALID          TargetType = "INVALID"
	TARGET_TYPE_INFERENCE_SERVER TargetType = "INFERENCE_SERVER"
	TARGET_TYPE_OFFLINE          TargetType = "OFFLINE"
	TARGET_TYPE_MOBILE           TargetType = "MOBILE"
	TARGET_TYPE_SELF_HOSTED      TargetType = "SELF_HOSTED"
)

// DeploymentStage enum for deployment stages
type DeploymentStage string

const (
	DEPLOYMENT_STAGE_INVALID              DeploymentStage = "INVALID"
	DEPLOYMENT_STAGE_VALIDATION           DeploymentStage = "VALIDATION"
	DEPLOYMENT_STAGE_RESOURCE_ACQUISITION DeploymentStage = "RESOURCE_ACQUISITION"
	DEPLOYMENT_STAGE_PLACEMENT            DeploymentStage = "PLACEMENT"
	DEPLOYMENT_STAGE_ROLLOUT_COMPLETE     DeploymentStage = "ROLLOUT_COMPLETE"
	DEPLOYMENT_STAGE_ROLLOUT_FAILED       DeploymentStage = "ROLLOUT_FAILED"
	DEPLOYMENT_STAGE_ROLLBACK_IN_PROGRESS DeploymentStage = "ROLLBACK_IN_PROGRESS"
	DEPLOYMENT_STAGE_ROLLBACK_COMPLETE    DeploymentStage = "ROLLBACK_COMPLETE"
	DEPLOYMENT_STAGE_ROLLBACK_FAILED      DeploymentStage = "ROLLBACK_FAILED"
	DEPLOYMENT_STAGE_CLEAN_UP_IN_PROGRESS DeploymentStage = "CLEAN_UP_IN_PROGRESS"
	DEPLOYMENT_STAGE_CLEAN_UP_COMPLETE    DeploymentStage = "CLEAN_UP_COMPLETE"
	DEPLOYMENT_STAGE_CLEAN_UP_FAILED      DeploymentStage = "CLEAN_UP_FAILED"
)

// Use protobuf ConditionStatus instead of local definition
// ConditionStatus values: api.CONDITION_STATUS_UNKNOWN, api.CONDITION_STATUS_TRUE, api.CONDITION_STATUS_FALSE

// ModelRevision represents a model revision
type ModelRevision struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// TargetDefinition defines the target for deployment
type TargetDefinition struct {
	Type    TargetType `json:"type,omitempty"`
	SubType string     `json:"subType,omitempty"`
}

// DeletionSpec specifies deletion settings
type DeletionSpec struct {
	Deleted bool `json:"deleted,omitempty"`
}

// BlastUpdate represents a blast update strategy
type BlastUpdate struct {
	WithRollbackTrigger bool   `json:"withRollbackTrigger,omitempty"`
	JiraLink            string `json:"jiraLink,omitempty"`
}

// DeploymentStrategy represents the deployment strategy
type DeploymentStrategy struct {
	Blast *BlastUpdate `json:"blast,omitempty"`
}

// InferenceServerSpec represents inference server specifications
type InferenceServerSpec struct {
	// Add fields as needed
}

// DeploymentSpec defines the desired state of Deployment
type DeploymentSpec struct {
	DesiredRevision *ModelRevision       `json:"desiredRevision,omitempty"`
	Definition      *TargetDefinition    `json:"definition,omitempty"`
	DeletionSpec    *DeletionSpec        `json:"deletionSpec,omitempty"`
	Strategy        *DeploymentStrategy  `json:"strategy,omitempty"`
	InferenceServer *InferenceServerSpec `json:"inferenceServer,omitempty"`
}

// Use the protobuf Condition type instead of local definition
// type Condition = api.Condition

// DeploymentStatus defines the observed state of Deployment
type DeploymentStatus struct {
	Stage              DeploymentStage  `json:"stage,omitempty"`
	Message            string           `json:"message,omitempty"`
	CurrentRevision    *ModelRevision   `json:"currentRevision,omitempty"`
	CandidateRevision  *ModelRevision   `json:"candidateRevision,omitempty"`
	Conditions         []*api.Condition `json:"conditions,omitempty"`
	ConditionsSnapshot []*api.Condition `json:"conditionsSnapshot,omitempty"`
	ProviderStatus     string           `json:"providerStatus,omitempty"`
}

// Deployment is the Schema for the deployments API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentSpec   `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

// DeploymentList contains a list of Deployment
// +kubebuilder:object:root=true
type DeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployment `json:"items"`
}

// DeploymentEventSpec defines the spec for deployment events
type DeploymentEventSpec struct {
	Content interface{} `json:"content,omitempty"`
}

// DeploymentEvent represents an event in deployment history
// +kubebuilder:object:root=true
type DeploymentEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DeploymentEventSpec `json:"spec,omitempty"`
}

// Helper methods

// GetDefinition returns the definition
func (d *DeploymentSpec) GetDefinition() *TargetDefinition {
	return d.Definition
}

// GetDesiredRevision returns the desired revision
func (d *DeploymentSpec) GetDesiredRevision() *ModelRevision {
	return d.DesiredRevision
}

// GetName returns the name
func (m *ModelRevision) GetName() string {
	if m == nil {
		return ""
	}
	return m.Name
}

// GetCandidateRevision returns the candidate revision
func (d *DeploymentStatus) GetCandidateRevision() *ModelRevision {
	return d.CandidateRevision
}

// GetCurrentRevision returns the current revision
func (d *DeploymentStatus) GetCurrentRevision() *ModelRevision {
	return d.CurrentRevision
}

// GetConditions returns the conditions
func (d *DeploymentStatus) GetConditions() []*api.Condition {
	return d.Conditions
}

// GetType returns the target type
func (t *TargetDefinition) GetType() TargetType {
	if t == nil {
		return TARGET_TYPE_INVALID
	}
	return t.Type
}

// String returns string representation of TargetType
func (t TargetType) String() string {
	return string(t)
}

// GetStrategy returns the strategy
func (d *DeploymentSpec) GetStrategy() *DeploymentStrategy {
	return d.Strategy
}

// GetBlast returns the blast strategy
func (s *DeploymentStrategy) GetBlast() *BlastUpdate {
	if s == nil {
		return nil
	}
	return s.Blast
}

// GetJiraLink returns the jira link
func (b *BlastUpdate) GetJiraLink() string {
	if b == nil {
		return ""
	}
	return b.JiraLink
}

// GetWithRollbackTrigger returns whether rollback trigger is enabled
func (b *BlastUpdate) GetWithRollbackTrigger() bool {
	if b == nil {
		return false
	}
	return b.WithRollbackTrigger
}

// DeepCopy creates a deep copy of the deployment
func (d *Deployment) DeepCopy() *Deployment {
	if d == nil {
		return nil
	}
	out := &Deployment{}
	d.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the deployment into the provided deployment
func (d *Deployment) DeepCopyInto(out *Deployment) {
	*out = *d
	out.TypeMeta = d.TypeMeta
	d.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	d.Spec.DeepCopyInto(&out.Spec)
	d.Status.DeepCopyInto(&out.Status)
}

// DeepCopyInto copies the spec
func (d *DeploymentSpec) DeepCopyInto(out *DeploymentSpec) {
	*out = *d
	if d.DesiredRevision != nil {
		out.DesiredRevision = &ModelRevision{
			Name:      d.DesiredRevision.Name,
			Namespace: d.DesiredRevision.Namespace,
		}
	}
	if d.Definition != nil {
		out.Definition = &TargetDefinition{
			Type:    d.Definition.Type,
			SubType: d.Definition.SubType,
		}
	}
	if d.DeletionSpec != nil {
		out.DeletionSpec = &DeletionSpec{
			Deleted: d.DeletionSpec.Deleted,
		}
	}
	// Add other fields as needed
}

// DeepCopyInto copies the status
func (d *DeploymentStatus) DeepCopyInto(out *DeploymentStatus) {
	*out = *d
	if d.CurrentRevision != nil {
		out.CurrentRevision = &ModelRevision{
			Name:      d.CurrentRevision.Name,
			Namespace: d.CurrentRevision.Namespace,
		}
	}
	if d.CandidateRevision != nil {
		out.CandidateRevision = &ModelRevision{
			Name:      d.CandidateRevision.Name,
			Namespace: d.CandidateRevision.Namespace,
		}
	}
	// Copy conditions slices
	if d.Conditions != nil {
		out.Conditions = make([]*api.Condition, len(d.Conditions))
		for i, condition := range d.Conditions {
			if condition != nil {
				out.Conditions[i] = &api.Condition{
					Type:                 condition.Type,
					Status:               condition.Status,
					Reason:               condition.Reason,
					Message:              condition.Message,
					LastUpdatedTimestamp: condition.LastUpdatedTimestamp,
				}
			}
		}
	}
	if d.ConditionsSnapshot != nil {
		out.ConditionsSnapshot = make([]*api.Condition, len(d.ConditionsSnapshot))
		for i, condition := range d.ConditionsSnapshot {
			if condition != nil {
				out.ConditionsSnapshot[i] = &api.Condition{
					Type:                 condition.Type,
					Status:               condition.Status,
					Reason:               condition.Reason,
					Message:              condition.Message,
					LastUpdatedTimestamp: condition.LastUpdatedTimestamp,
				}
			}
		}
	}
}

// DeepCopyObject returns a deep copy
func (d *Deployment) DeepCopyObject() runtime.Object {
	if c := d.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy creates a deep copy of the deployment list
func (d *DeploymentList) DeepCopy() *DeploymentList {
	if d == nil {
		return nil
	}
	out := &DeploymentList{}
	d.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the deployment list into the provided deployment list
func (d *DeploymentList) DeepCopyInto(out *DeploymentList) {
	*out = *d
	out.TypeMeta = d.TypeMeta
	d.ListMeta.DeepCopyInto(&out.ListMeta)
	if d.Items != nil {
		out.Items = make([]Deployment, len(d.Items))
		for i := range d.Items {
			d.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopyObject returns a deep copy
func (d *DeploymentList) DeepCopyObject() runtime.Object {
	if c := d.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy creates a deep copy of the deployment event
func (d *DeploymentEvent) DeepCopy() *DeploymentEvent {
	if d == nil {
		return nil
	}
	out := &DeploymentEvent{}
	d.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the deployment event into the provided deployment event
func (d *DeploymentEvent) DeepCopyInto(out *DeploymentEvent) {
	*out = *d
	out.TypeMeta = d.TypeMeta
	d.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	// For simplicity, we'll do a shallow copy of the spec content
	out.Spec = d.Spec
}

// DeepCopyObject returns a deep copy
func (d *DeploymentEvent) DeepCopyObject() runtime.Object {
	if c := d.DeepCopy(); c != nil {
		return c
	}
	return nil
}
