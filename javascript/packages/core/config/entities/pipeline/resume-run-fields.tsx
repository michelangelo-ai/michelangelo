import { useMemo, useState } from 'react';
import { create } from '@bufbuild/protobuf';
import { anyPack, StringValueSchema } from '@bufbuild/protobuf/wkt';

import { SelectField } from '#core/components/form/fields/select/select-field';
import { useField } from '#core/components/form/hooks/use-field';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { RUN_STATE_TEXT_MAP } from '#core/config/entities/run/shared';
import { TERMINAL_RUN_STATES } from '#core/config/entities/run/types';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { timestampToString } from '#core/utils/time-utils';
import { ResumeStepOption } from './resume-step-option';

import type { SelectOption } from '#core/components/form/fields/select/types';
import type {
  GetPipelineRunResponse,
  ListPipelineRunResponse,
  PipelineRunStepInfo,
  PipelineRunSummary,
} from '#core/config/entities/run/types';

export const SOURCE_RUN_FIELD = 'spec.resume.pipelineRun.name';
export const RESUME_FROM_FIELD = 'spec.resume.resumeFrom';

/**
 * Name of the step whose sub-steps are the pipeline's DAG tasks. Its siblings
 * (Source Pipeline, Image Build) are platform stages and are not resumable.
 * Mirrors `ExecuteWorkflowStepName` in `go/components/pipelinerun/actors/utils/utils.go`.
 */
const EXECUTE_WORKFLOW_STEP_NAME = 'Execute Workflow';

/** Keeps the picker scannable; older runs stay reachable through search. */
const MAX_SOURCE_RUN_OPTIONS = 25;

/** `CRITERION_OPERATOR_EQUAL` from `proto/api/list.proto`. */
const CRITERION_OPERATOR_EQUAL = 1;

/**
 * Narrows the run list to one pipeline server-side.
 *
 * `pipeline_name` is the column the metadata store generates from the `spec.pipeline`
 * index annotation on `PipelineRun`, and it is indexed alongside `pipeline_namespace`.
 * Note the flat column form rather than the dotted proto path used elsewhere
 * (`pipeline_run.spec.actor.name`): criterion field names are resolved straight to SQL
 * columns, and a name still containing dots after the CRD prefix is stripped is rejected.
 *
 * This narrows the query but cannot be relied on alone — see {@link buildSourceRunOptions}.
 */
function buildPipelineCriterion(pipelineName: string) {
  return {
    listOptionsExt: {
      operation: {
        criterion: [
          {
            fieldName: 'pipeline_run.pipeline_name',
            operator: CRITERION_OPERATOR_EQUAL,
            matchValue: anyPack(
              StringValueSchema,
              create(StringValueSchema, { value: pipelineName })
            ),
          },
        ],
      },
    },
  };
}

/**
 * Collapsible group for continuing a previous pipeline run.
 *
 * Collapsed, it costs nothing — neither query runs until the user opens it. Expanding
 * lists the pipeline's finished runs; picking one loads that run's DAG tasks into the
 * step picker.
 *
 * Renders inside a `Form`, and reads the selected source run straight from form state
 * rather than duplicating it in local state.
 */
export const ResumeRunFields = ({ pipelineName }: { pipelineName: string }) => {
  const [expanded, setExpanded] = useState(false);

  const handleResumeGroupToggle = (isExpanded: boolean) => setExpanded(isExpanded);

  const { input: sourceRunInput } = useField<string>(SOURCE_RUN_FIELD);
  const sourceRunName = sourceRunInput.value;

  const { data: runListData, isLoading: isLoadingRuns } = useStudioQuery<ListPipelineRunResponse>({
    queryName: 'ListPipelineRun',
    serviceOptions: buildPipelineCriterion(pipelineName),
    clientOptions: { enabled: expanded },
  });

  const { data: sourceRunData, isLoading: isLoadingSteps } = useStudioQuery<GetPipelineRunResponse>(
    {
      queryName: 'GetPipelineRun',
      serviceOptions: { name: sourceRunName },
      clientOptions: { enabled: !!sourceRunName },
    }
  );

  const sourceRunOptions = useMemo(
    () => buildSourceRunOptions(runListData?.pipelineRunList?.items, pipelineName),
    [runListData, pipelineName]
  );

  const steps = useMemo(() => getResumableSteps(sourceRunData?.pipelineRun), [sourceRunData]);

  const stepOptions = useMemo<SelectOption<string>[]>(
    () =>
      steps.map((step) => ({
        id: step.displayName ?? '',
        label: step.displayName ?? '',
      })),
    [steps]
  );

  const stepsById = useMemo(() => {
    const map = new Map<string, PipelineRunStepInfo>();
    for (const step of steps) {
      if (step.displayName) map.set(step.displayName, step);
    }
    return map;
  }, [steps]);

  const hasSourceRun = !!sourceRunName;
  const hasNoSteps = hasSourceRun && !isLoadingSteps && steps.length === 0;

  return (
    <FormGroup
      collapsible
      title="Select run to resume from"
      description="Continue a previous run instead of starting over. Completed tasks are reused from the source run."
      onToggle={handleResumeGroupToggle}
    >
      <SelectField
        name={SOURCE_RUN_FIELD}
        label="Pipeline run"
        placeholder="Search runs…"
        options={sourceRunOptions}
        isLoading={isLoadingRuns}
        caption={
          !isLoadingRuns && sourceRunOptions.length === 0
            ? 'This pipeline has no finished runs to resume from.'
            : undefined
        }
      />

      <SelectField<string>
        multi
        name={RESUME_FROM_FIELD}
        label="Steps"
        placeholder={hasSourceRun ? 'Search steps…' : 'Select a pipeline run first'}
        options={stepOptions}
        isLoading={isLoadingSteps}
        disabled={!hasSourceRun || hasNoSteps}
        creatable={false}
        getOptionContent={(option) => <ResumeStepOption step={stepsById.get(option.id)} />}
        caption={
          hasNoSteps
            ? 'This run has no recorded workflow tasks to resume from.'
            : 'Leave empty to continue from where the source run stopped, reusing all cached outputs.'
        }
      />
    </FormGroup>
  );
};

/**
 * Finished runs of this pipeline, newest first.
 *
 * The pipeline filter is repeated here even though {@link buildPipelineCriterion} already
 * asks for it server-side: `ListOptionsExt` is honored only when metadata storage is
 * enabled and is otherwise silently ignored, so relying on it alone would widen the picker
 * to other pipelines' runs rather than narrowing it. (The label selector the pipeline
 * detail page uses, `pipeline.michelangelo/name`, is written by nothing in the platform,
 * so it is not an alternative.) This pass also does the terminal-state gating, newest-first
 * ordering, and cap, none of which the criterion expresses.
 */
function buildSourceRunOptions(
  items: PipelineRunSummary[] | undefined,
  pipelineName: string
): SelectOption<string>[] {
  if (!items) return [];

  return items
    .filter(
      (run) =>
        run.spec?.pipeline?.name === pipelineName &&
        run.status?.state !== undefined &&
        TERMINAL_RUN_STATES.has(run.status.state) &&
        !!run.metadata?.name
    )
    .sort(
      (a, b) =>
        Number(b.metadata?.creationTimestamp?.seconds ?? 0) -
        Number(a.metadata?.creationTimestamp?.seconds ?? 0)
    )
    .slice(0, MAX_SOURCE_RUN_OPTIONS)
    .map((run) => ({
      id: run.metadata?.name ?? '',
      label: buildSourceRunLabel(run),
    }));
}

function buildSourceRunLabel(run: PipelineRunSummary): string {
  const state = RUN_STATE_TEXT_MAP[run.status?.state ?? 0] ?? 'Unknown';
  const created = timestampToString(run.metadata?.creationTimestamp?.seconds);

  return [run.metadata?.name, state, created].filter(Boolean).join(' · ');
}

/**
 * The DAG tasks of a run.
 *
 * Values submitted as `resumeFrom` must be sub-step `displayName`s: the platform keys its
 * task cache by display name, so submitting `name` (which holds the task path) produces a
 * run that silently reuses the cache for the very step the user asked to re-run.
 */
function getResumableSteps(run: PipelineRunSummary | undefined): PipelineRunStepInfo[] {
  const executeWorkflowStep = run?.status?.steps?.find(
    (step) => step.name === EXECUTE_WORKFLOW_STEP_NAME
  );

  return (executeWorkflowStep?.subSteps ?? []).filter((step) => !!step.displayName);
}
