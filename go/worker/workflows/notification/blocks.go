package notification

import (
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	v2pb "github.com/michelangelo-ai/michelangelo/proto-go/api/v2"
)

const (
	_maPipelineNamePrefix = "pipeline-"
	_truncateMaxLen       = 2900
)

// GenerateBlocks returns Slack Block Kit blocks for the given pipeline run state.
// Returns nil for states that don't have Block Kit representations.
//
// To customize the console URL, inject a custom types.PhaseResolver that returns
// the desired base URL via GetStudioBaseURL or override this function in your fork.
func GenerateBlocks(pipelineRun *v2pb.PipelineRun, studioBaseURL string) []slack.Block {
	if pipelineRun == nil {
		return nil
	}

	state := pipelineRun.Status.State
	consoleURL := buildConsoleURL(pipelineRun, studioBaseURL)
	pipelineName := strings.TrimPrefix(pipelineRun.Spec.Pipeline.GetName(), _maPipelineNamePrefix)
	runID := pipelineRun.Name
	workspace := pipelineRun.Namespace

	field := func(label, value string) *slack.TextBlockObject {
		return slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*%s:*\n`%s`", label, value), false, false)
	}

	viewBtn := slack.NewButtonBlockElement("view_console", "", slack.NewTextBlockObject(slack.PlainTextType, "View Details", false, false))
	if consoleURL != "" {
		viewBtn.URL = consoleURL
	}

	switch state {
	case v2pb.PIPELINE_RUN_STATE_RUNNING:
		initiatedAt := ""
		if !pipelineRun.CreationTimestamp.IsZero() {
			initiatedAt = pipelineRun.CreationTimestamp.UTC().Format("2006-01-02T15:04:05.000Z")
		}
		trigger := "scheduled"
		if pipelineRun.Spec.Actor != nil && pipelineRun.Spec.Actor.Name != "" {
			trigger = "manual"
		}
		fields := []*slack.TextBlockObject{
			field("Pipeline", pipelineName),
			field("Pipeline Run ID", runID),
			field("Trigger", trigger),
			field("Workspace", workspace),
		}
		if initiatedAt != "" {
			fields = append(fields, field("Initiated At", initiatedAt))
		}
		return []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, "Pipeline run started", true, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "A pipeline run was triggered. See details below:", false, false), nil, nil),
			slack.NewSectionBlock(nil, fields, nil),
			slack.NewDividerBlock(),
			slack.NewActionBlock("", viewBtn),
		}

	case v2pb.PIPELINE_RUN_STATE_KILLED:
		cancelledBy := ""
		if pipelineRun.Spec.Actor != nil {
			cancelledBy = pipelineRun.Spec.Actor.Name
		}
		fields := []*slack.TextBlockObject{
			field("Pipeline", pipelineName),
			field("Pipeline Run ID", runID),
			field("Workspace", workspace),
		}
		if cancelledBy != "" {
			fields = append(fields, field("Cancelled by", cancelledBy))
		}
		return []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, "Pipeline run cancelled", true, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "A pipeline run was cancelled. See details below:", false, false), nil, nil),
			slack.NewSectionBlock(nil, fields, nil),
			slack.NewDividerBlock(),
			slack.NewActionBlock("", viewBtn),
		}

	case v2pb.PIPELINE_RUN_STATE_FAILED:
		failedTaskName, failedTaskMsg := findFailedTask(pipelineRun.Status.Steps)
		fields := []*slack.TextBlockObject{
			field("Pipeline", pipelineName),
			field("Pipeline Run ID", runID),
			field("Workspace", workspace),
		}
		if failedTaskName != "" {
			fields = append(fields, field("Task", failedTaskName))
		}
		blocks := []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, "❌ Pipeline run failed", true, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "A pipeline run failed. See details below:", false, false), nil, nil),
			slack.NewSectionBlock(nil, fields, nil),
			slack.NewDividerBlock(),
		}
		errMsg := failedTaskMsg
		if errMsg == "" {
			errMsg = pipelineRun.Status.ErrorMessage
		}
		if errMsg != "" {
			blocks = append(blocks,
				slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "*Error:*", false, false), nil, nil),
				slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "```"+truncateError(errMsg)+"```", false, false), nil, nil),
				slack.NewDividerBlock(),
			)
		}
		blocks = append(blocks, slack.NewActionBlock("", viewBtn))
		return blocks

	case v2pb.PIPELINE_RUN_STATE_SUCCEEDED:
		return []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, "✅ Pipeline run completed", true, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, "A pipeline run completed. See details below:", false, false), nil, nil),
			slack.NewSectionBlock(nil, []*slack.TextBlockObject{
				field("Pipeline", pipelineName),
				field("Pipeline Run ID", runID),
				field("Workspace", workspace),
			}, nil),
			slack.NewDividerBlock(),
			slack.NewActionBlock("", viewBtn),
		}

	default:
		return nil
	}
}

// buildConsoleURL builds the console URL for the pipeline run.
// Returns empty string if studioBaseURL is not provided.
func buildConsoleURL(pipelineRun *v2pb.PipelineRun, studioBaseURL string) string {
	if studioBaseURL == "" {
		return ""
	}
	pipelineID := ""
	if pipelineRun.Spec.Pipeline != nil {
		pipelineID = strings.TrimPrefix(pipelineRun.Spec.Pipeline.Name, _maPipelineNamePrefix)
	}
	return fmt.Sprintf("%s/workspace/%s/pipeline/%s/runs", studioBaseURL, pipelineRun.Namespace, pipelineID)
}

// findFailedTask returns the name and error message of the first failed step, searching substeps recursively.
func findFailedTask(steps []*v2pb.PipelineRunStepInfo) (name, message string) {
	for _, step := range steps {
		if step.State == v2pb.PIPELINE_RUN_STEP_STATE_FAILED {
			if subName, subMsg := findFailedTask(step.SubSteps); subName != "" {
				return subName, subMsg
			}
			return step.Name, step.Message
		}
	}
	return "", ""
}

func truncateError(msg string) string {
	if len(msg) <= _truncateMaxLen {
		return msg
	}
	return "...(truncated)\n" + msg[len(msg)-_truncateMaxLen:]
}