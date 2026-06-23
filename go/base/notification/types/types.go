// Package types provides shared types and utility functions for pipeline run notifications.
//
// This package is the single source of truth for notification constants, request
// structures, and message-generation helpers. It intentionally has no imports
// from worker or controller packages to avoid circular dependencies.
package types

import (
	"fmt"
	"strings"

	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PipelineRunNotificationWorkflowName is the registered name of the pipeline run
	// notification workflow in Cadence/Temporal. Both the notifier (which starts the
	// workflow) and the worker (which registers it) must use this constant.
	//
	// The name uses a reverse-DNS prefix to avoid collisions in shared Cadence/Temporal
	// namespaces where multiple teams or services register workflows side by side.
	// Temporal does not mandate this convention, but it is a widely used practice for
	// the same reason Java uses it for package names.
	PipelineRunNotificationWorkflowName = "io.michelangelo.notification.PipelineRunFanout"

	// DeprecatedPRNotificationWorkflowName is the workflow name used before this
	// package was open-sourced. The worker registers this alias for one release so
	// that in-flight executions dispatched by a pre-upgrade controllermgr can drain
	// without hanging until their 60h ExecutionStartToCloseTimeout. Remove once all
	// operators have rolled past this release.
	DeprecatedPRNotificationWorkflowName = "PRNotificationWorkflow"

	// sourcePipelineTypeLabelName is the Kubernetes label key that identifies the
	// pipeline type (e.g. PIPELINE_TYPE_TRAIN).
	sourcePipelineTypeLabelName = "michelangelo/SourcePipelineType"
	// sourcePipelineManifestTypeLabelName is the Kubernetes label key that identifies
	// the pipeline manifest type (e.g. PIPELINE_MANIFEST_TYPE_ASL).
	sourcePipelineManifestTypeLabelName = "pipeline.michelangelo/PipelineManifestType"

	// pipelineManifestTypeASL identifies ASL (Amazon States Language) pipelines.
	// These use Cadence as the workflow engine, so the notification includes a
	// workflow log URL that is not relevant for other pipeline types.
	pipelineManifestTypeASL = "PIPELINE_MANIFEST_TYPE_ASL"
)

// PhaseResolver maps a pipeline type label value to a UI path segment used when
// building deep links in notification messages.
//
// Operators with custom pipeline types should supply their own resolver rather
// than relying on DefaultPhaseResolver, which only covers the built-in types.
type PhaseResolver func(pipelineType string) string

// DefaultPhaseResolver maps the built-in pipeline type label values to their
// corresponding UI path segments.
func DefaultPhaseResolver(pipelineType string) string {
	switch pipelineType {
	case "PIPELINE_TYPE_TRAIN", "PIPELINE_TYPE_EVAL":
		return "train"
	case "PIPELINE_TYPE_SCORER", "PIPELINE_TYPE_PREDICTION":
		return "deploy"
	case "PIPELINE_TYPE_RETRAIN", "PIPELINE_TYPE_EXPERIMENT", "PIPELINE_TYPE_POST_PROCESSING", "PIPELINE_TYPE_OPTIMIZATION":
		return "retrain"
	case "PIPELINE_TYPE_PERF_EVAL", "PIPELINE_TYPE_PERFORMANCE_MONITORING",
		"PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY", "PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION":
		return "monitor"
	case "PIPELINE_TYPE_DATA_PREP", "PIPELINE_TYPE_BASIS_FEATURE":
		return "data"
	case "PIPELINE_TYPE_EMBEDDING_GENERATION", "PIPELINE_TYPE_EMBEDDING_GENERATION_ORCHESTRATION":
		return "genai-data"
	case "PIPELINE_TYPE_TRAIN_LLM", "PIPELINE_TYPE_EVAL_LLM":
		return "genai-finetune"
	case "PIPELINE_TYPE_EVAL_PROMPT", "PIPELINE_TYPE_LLM_ONE_OFF_GENERATION",
		"PIPELINE_TYPE_LLM_ONE_OFF_GENERATION_ORCHESTRATION":
		return "genai-prompt"
	default:
		return "pipeline"
	}
}

// PipelineRunNotificationRequest is the serializable payload passed to the
// PipelineRunNotificationWorkflow. All fields must be JSON-serializable because
// Cadence/Temporal persists them as workflow input.
type PipelineRunNotificationRequest struct {
	// PipelineRun is the cropped pipeline run that triggered the notification.
	// Use CropPipelineRun before populating this field to keep the payload small.
	PipelineRun *v2pb.PipelineRun `json:"pipeline_run"`
	// StudioBaseURL is the base URL of the platform UI, used to build deep links
	// in notification bodies. Example: "https://ml.mycompany.com/studio/".
	// If empty, no deep link is included in the message.
	StudioBaseURL string `json:"studio_base_url"`
	// SenderEmail is the From address for outgoing email notifications.
	// If empty, the activity implementation chooses its own default.
	SenderEmail string `json:"sender_email"`
}

// ContainsEventType reports whether any of eventTypes corresponds to prState.
func ContainsEventType(eventTypes []v2pb.Notification_EventType, prState v2pb.PipelineRunState) bool {
	// PIPELINE_RUN_STATE_RUNNING maps to EVENT_TYPE_PIPELINE_RUN_STATE_STARTED:
	// from a user perspective the run has "started" when it enters the RUNNING
	// state. The event name reflects the lifecycle milestone, not the internal
	// state constant name.
	stateMap := map[v2pb.PipelineRunState]v2pb.Notification_EventType{
		v2pb.PIPELINE_RUN_STATE_FAILED:    v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED,
		v2pb.PIPELINE_RUN_STATE_SUCCEEDED: v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
		v2pb.PIPELINE_RUN_STATE_KILLED:    v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_KILLED,
		v2pb.PIPELINE_RUN_STATE_SKIPPED:   v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED,
		v2pb.PIPELINE_RUN_STATE_RUNNING:   v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED,
	}
	mapped, ok := stateMap[prState]
	if !ok {
		return false
	}
	for _, et := range eventTypes {
		// Accept both STARTED (new) and deprecated RUNNING for backward
		// compatibility with currently deployed systems.
		if et == mapped || (mapped == v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_STARTED && et == v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_RUNNING) {
			return true
		}
	}
	return false
}

// GenerateSubject returns the email subject line for a pipeline run notification.
func GenerateSubject(pipelineRun *v2pb.PipelineRun) string {
	state := strings.TrimPrefix(pipelineRun.Status.State.String(), "PIPELINE_RUN_STATE_")
	return fmt.Sprintf("Pipeline Run (%s) state: %s", pipelineRun.Name, state)
}

// GenerateBody returns a channel-agnostic plain-text notification body suitable
// for any delivery channel. Channel-specific formatting (e.g. Slack mrkdwn)
// should use GenerateText with the appropriate notification type instead.
func GenerateBody(pipelineRun *v2pb.PipelineRun, studioBaseURL string, phaseResolver PhaseResolver) string {
	if phaseResolver == nil {
		phaseResolver = DefaultPhaseResolver
	}
	pipelineType := pipelineRun.Labels[sourcePipelineTypeLabelName]
	pipelineManifestType := pipelineRun.Labels[sourcePipelineManifestTypeLabelName]
	state := strings.TrimPrefix(pipelineRun.Status.State.String(), "PIPELINE_RUN_STATE_")
	pipelineTypeStr := strings.TrimPrefix(pipelineType, "PIPELINE_TYPE_")

	var studioLink string
	if studioBaseURL != "" {
		base := strings.TrimRight(studioBaseURL, "/") + "/"
		phase := phaseResolver(pipelineType)
		studioLink = fmt.Sprintf("%s%s/%s/runs/%s", base, pipelineRun.Namespace, phase, pipelineRun.Name)
	}

	text := fmt.Sprintf("Pipeline Run Status Update:\n- Name: %s\n- Project: %s\n- State: %s\n- Pipeline Type: %s\n",
		pipelineRun.Name, pipelineRun.Namespace, state, pipelineTypeStr)
	if studioLink != "" {
		text += fmt.Sprintf("- Studio URL: %s\n", studioLink)
	}
	if pipelineManifestType == pipelineManifestTypeASL && pipelineRun.Status.LogUrl != "" {
		text += fmt.Sprintf("- Workflow Log URL: %s\n", pipelineRun.Status.LogUrl)
	}
	return text
}

// GenerateText returns a channel-specific notification body.
//
// textType must be v2pb.NOTIFICATION_TYPE_EMAIL or v2pb.NOTIFICATION_TYPE_SLACK.
// For EMAIL, this delegates to GenerateBody (plain text). For SLACK, it produces
// Slack mrkdwn with clickable <url|label> links.
//
// studioBaseURL is the base URL of the platform UI used to build a deep link.
// A trailing slash is added automatically if missing. Pass an empty string to
// omit the link entirely.
//
// phaseResolver maps the pipeline type label value to a UI path segment.
// Pass nil to use DefaultPhaseResolver. Implement PhaseResolver to customize
// path segments for non-standard or operator-defined pipeline types.
func GenerateText(pipelineRun *v2pb.PipelineRun, textType v2pb.Notification_NotificationType, studioBaseURL string, phaseResolver PhaseResolver) string {
	// Non-SLACK types (including EMAIL and INVALID) get the plain-text body.
	if textType != v2pb.NOTIFICATION_TYPE_SLACK {
		return GenerateBody(pipelineRun, studioBaseURL, phaseResolver)
	}

	if phaseResolver == nil {
		phaseResolver = DefaultPhaseResolver
	}
	pipelineType := pipelineRun.Labels[sourcePipelineTypeLabelName]
	pipelineManifestType := pipelineRun.Labels[sourcePipelineManifestTypeLabelName]
	state := strings.TrimPrefix(pipelineRun.Status.State.String(), "PIPELINE_RUN_STATE_")
	pipelineTypeStr := strings.TrimPrefix(pipelineType, "PIPELINE_TYPE_")

	var studioLink string
	if studioBaseURL != "" {
		base := strings.TrimRight(studioBaseURL, "/") + "/"
		phase := phaseResolver(pipelineType)
		studioLink = fmt.Sprintf("%s%s/%s/runs/%s", base, pipelineRun.Namespace, phase, pipelineRun.Name)
	}

	text := fmt.Sprintf("%s:\n- Name: %s\n- Project: %s\n- State: %s\n- Pipeline Type: %s\n",
		GenerateSubject(pipelineRun), pipelineRun.Name, pipelineRun.Namespace, state, pipelineTypeStr)
	if studioLink != "" {
		text += fmt.Sprintf("- <%s|Studio URL>\n", studioLink)
	}
	if pipelineManifestType == pipelineManifestTypeASL && pipelineRun.Status.LogUrl != "" {
		text += fmt.Sprintf("- <%s|Workflow Log URL>\n", pipelineRun.Status.LogUrl)
	}
	return text
}

// CropPipelineRun returns a copy of r with only the fields needed for notification
// delivery. Use this before passing a pipeline run to the notification workflow to
// stay within Cadence/Temporal payload size limits.
func CropPipelineRun(r *v2pb.PipelineRun) *v2pb.PipelineRun {
	if r == nil {
		return nil
	}
	status := r.Status
	return &v2pb.PipelineRun{
		TypeMeta: r.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   r.Namespace,
			Name:        r.Name,
			Labels:      r.Labels,
			Annotations: r.Annotations,
		},
		Spec: r.Spec,
		Status: v2pb.PipelineRunStatus{
			State:        status.State,
			LogUrl:       status.LogUrl,
			ErrorMessage: status.ErrorMessage,
			Code:         status.Code,
			EndTime:      status.EndTime,
		},
	}
}
