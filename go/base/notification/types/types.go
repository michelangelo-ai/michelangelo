// Package types provides utility functions for pipeline run notifications.
//
// This package contains helper functions that match the internal Michelangelo
// implementation for consistency and easier import/export compatibility.
package types

import (
	"fmt"
	"strings"

	v2pb "github.com/michelangelo-ai/michelangelo/proto/api/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	_defaultMaURL   = "https://michelangelo-studio.uberinternal.com/ma/"
	_defaultMaEmail = "michelangelo@uber.com"
	_sourcePipelineTypeLabelName         = "michelangelo/SourcePipelineType"
	_sourcePipelineManifestTypeLabelName = "pipeline.michelangelo/PipelineManifestType"
)

// containsEventType checks if the given event types contain the pipeline run state.
func ContainsEventType(eventTypes []v2pb.Notification_EventType, prState v2pb.PipelineRunState) bool {
	var stateMap = map[v2pb.PipelineRunState]v2pb.Notification_EventType{
		v2pb.PIPELINE_RUN_STATE_FAILED:    v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_FAILED,
		v2pb.PIPELINE_RUN_STATE_SUCCEEDED: v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SUCCEEDED,
		v2pb.PIPELINE_RUN_STATE_KILLED:    v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_KILLED,
		v2pb.PIPELINE_RUN_STATE_SKIPPED:   v2pb.EVENT_TYPE_PIPELINE_RUN_STATE_SKIPPED,
	}
	for _, eventType := range eventTypes {
		if eventType == stateMap[prState] {
			return true
		}
	}
	return false
}

func GenerateSubject(pipelineRun *v2pb.PipelineRun) string {
	state := strings.TrimPrefix(pipelineRun.Status.State.String(), "PIPELINE_RUN_STATE_")
	return fmt.Sprintf("Pipeline Run (%s) has completed with state %s", pipelineRun.Name, state)
}

func GenerateText(pipelineRun *v2pb.PipelineRun, textType string) string {
	pipelineType := pipelineRun.Labels[_sourcePipelineTypeLabelName]
	pipelineManifestType := pipelineRun.Labels[_sourcePipelineManifestTypeLabelName]
	maURL := fmt.Sprintf(_defaultMaURL+"%s/%s/runs/%s", pipelineRun.Namespace, getPipelinePhase(pipelineType), pipelineRun.Name)
	state := strings.TrimPrefix(pipelineRun.Status.State.String(), "PIPELINE_RUN_STATE_")
	pipelineTypeStr := strings.TrimPrefix(pipelineType, "PIPELINE_TYPE_")

	if textType == "slack" {
		slackText := fmt.Sprintf("%s:\n- Name: %s\n- Project: %s\n- State: %s\n- Pipeline Type: %s\n- <%s|Michelangelo Studio URL>\n",
			GenerateSubject(pipelineRun), pipelineRun.Name, pipelineRun.Namespace, state, pipelineTypeStr, maURL)
		if pipelineManifestType == "PIPELINE_MANIFEST_TYPE_ASL" {
			slackText += fmt.Sprintf("- <%s|Cadence Log URL>\n", pipelineRun.Status.LogUrl)
		}
		return slackText
	}
	emailText := fmt.Sprintf("Your Michelangelo Studio Pipeline Run Has Status Update:\n- Name: %s\n- Project: %s\n- State: %s\n- Pipeline Type: %s\n- Michelangelo Studio URL: %s\n",
		pipelineRun.Name, pipelineRun.Namespace, state, pipelineTypeStr, maURL)
	if pipelineManifestType == "PIPELINE_MANIFEST_TYPE_ASL" {
		emailText += fmt.Sprintf("- Cadence Log URL: %s\n", pipelineRun.Status.LogUrl)
	}
	return emailText
}

func getPipelinePhase(pipelineType string) string {
	var maStudioPhase string
	switch pipelineType {
	case "PIPELINE_TYPE_TRAIN", "PIPELINE_TYPE_EVAL":
		maStudioPhase = "train"
	case "PIPELINE_TYPE_SCORER", "PIPELINE_TYPE_PREDICTION":
		maStudioPhase = "deploy"
	case "PIPELINE_TYPE_RETRAIN", "PIPELINE_TYPE_EXPERIMENT", "PIPELINE_TYPE_POST_PROCESSING", "PIPELINE_TYPE_OPTIMIZATION":
		maStudioPhase = "retrain"
	case "PIPELINE_TYPE_PERF_EVAL", "PIPELINE_TYPE_PERFORMANCE_MONITORING",
		"PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY", "PIPELINE_TYPE_ONLINE_OFFLINE_FEATURE_CONSISTENCY_ORCHESTRATION":
		maStudioPhase = "monitor"
	case "PIPELINE_TYPE_DATA_PREP", "PIPELINE_TYPE_BASIS_FEATURE":
		maStudioPhase = "data"
	case "PIPELINE_TYPE_EMBEDDING_GENERATION", "PIPELINE_TYPE_EMBEDDING_GENERATION_ORCHESTRATION":
		maStudioPhase = "genai-data"
	case "PIPELINE_TYPE_TRAIN_LLM", "PIPELINE_TYPE_EVAL_LLM":
		maStudioPhase = "genai-finetune"
	case "PIPELINE_TYPE_EVAL_PROMPT", "PIPELINE_TYPE_LLM_ONE_OFF_GENERATION",
		"PIPELINE_TYPE_LLM_ONE_OFF_GENERATION_ORCHESTRATION":
		maStudioPhase = "genai-prompt"
	default:
		maStudioPhase = "unknown"
	}
	return maStudioPhase
}

// CropPipelineRun crops the pipeline run to include the necessary fields for notifications to handle pipeline run size limit issues.
func CropPipelineRun(r *v2pb.PipelineRun) *v2pb.PipelineRun {
	if r == nil {
		return nil
	}
	status := r.Status
	res := &v2pb.PipelineRun{
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
	return res
}