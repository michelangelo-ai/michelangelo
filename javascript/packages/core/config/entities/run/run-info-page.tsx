import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { StringField } from '#core/components/form/fields/string/string-field';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { Form } from '#core/components/form/form';
import { LinksBox } from '#core/components/links-box/links-box';
import { TextEditor } from '#core/components/text-editor/text-editor';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { TimeZone } from '#core/types/time-types';
import { decodeStruct, isStruct } from '#core/utils/proto/struct-utils';
import { timestampToString } from '#core/utils/time-utils';
import { getRunManifestContent } from './shared';
import { PipelineRunState, TERMINAL_RUN_STATES } from './types';

import type { PipelineRunStepInfo, PipelineRunSummary, ReadOnlyField } from './types';

/**
 * Step name assigned by the backend to the wrapper step that executes the
 * workflow (`pipelinerunutils.ExecuteWorkflowStepName` in Go). Its `logUrl`
 * is the workflow-engine monitoring link for the whole run — the run-level
 * `status.logUrl` proto field exists but is never populated.
 */
const EXECUTE_WORKFLOW_STEP_NAME = 'Execute Workflow';

/** Label key stamped on every run by the create/update API hooks. */
const ENVIRONMENT_LABEL = 'michelangelo/environment';

/**
 * Logical execution time of a scheduled run, stamped by the trigger workflow as unix
 * seconds. Manual runs carry no such label.
 */
const EXECUTION_TIMESTAMP_LABEL = 'pipelinerun.michelangelo/execution-timestamp';

/**
 * Identifies which entry of the pipeline's `params_map` a scheduled run executed with.
 */
const PARAMETER_ID_LABEL = 'pipelinerun.michelangelo/parameter-id';

export function RunInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  const { projectId, phase } = useStudioParams('detail');

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const run = data as PipelineRunSummary | undefined;

  const resumedFromName = run?.spec?.resume?.pipelineRun?.name;
  const links = [
    {
      name: 'Michelangelo pipeline run logs',
      url: findStepByName(run?.status?.steps, EXECUTE_WORKFLOW_STEP_NAME)?.logUrl,
    },
    {
      name: resumedFromName ? `Resumed from ${resumedFromName}` : undefined,
      url: resumedFromName ? `/${projectId}/${phase}/runs/${resumedFromName}` : undefined,
    },
  ];

  const errorMessage = run?.status?.errorMessage;
  const manifestContent = getRunManifestContent(run);
  const input = decodeRunInput(run?.spec?.input);

  const statusFields: ReadOnlyField[] = [
    { id: 'duration', label: 'Duration', value: formatRunDuration(run) ?? '' },
    {
      id: 'execution-timestamp',
      label: 'Execution Timestamp',
      value: timestampToString(getExecutionTimestampSeconds(run), TimeZone.Local) ?? '',
    },
  ];

  const configurationFields: ReadOnlyField[] = [
    {
      id: 'environment',
      label: 'Environment',
      value: run?.metadata?.labels?.[ENVIRONMENT_LABEL] ?? '',
    },
    {
      id: 'parameter-id',
      label: 'Parameter ID',
      value: run?.metadata?.labels?.[PARAMETER_ID_LABEL] ?? '',
    },
  ];

  const column = css({
    display: 'flex',
    flexDirection: 'column',
    gap: theme.sizing.scale600,
  });

  const renderReadOnlyFields = (fields: ReadOnlyField[]) => (
    <Form
      onSubmit={() => undefined}
      initialValues={Object.fromEntries(fields.map((field) => [field.id, field.value]))}
    >
      <div className={column}>
        {fields.map((field) =>
          isLoading ? (
            <Skeleton key={field.id} animation height="48px" width="100%" />
          ) : (
            <StringField key={field.id} name={field.id} label={field.label} readOnly />
          )
        )}
      </div>
    </Form>
  );

  const renderJsonBox = (title: string, value: unknown) =>
    isLoading ? (
      <Skeleton animation height="200px" width="100%" />
    ) : (
      <Box title={title}>
        <TextEditor
          value={JSON.stringify(value, null, 2)}
          language="json"
          readOnly
          foldable
          height="auto"
        />
      </Box>
    );

  return (
    <div className={column}>
      <LinksBox title="Useful links" links={links} isLoading={isLoading} />

      {errorMessage && !isLoading && (
        <section>
          <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
            Message
          </HeadingSmall>
          <Box>
            <Form onSubmit={() => undefined} initialValues={{ 'error-message': errorMessage }}>
              <TextareaField name="error-message" label="Error message" readOnly />
            </Form>
          </Box>
        </section>
      )}

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Key status indicators
        </HeadingSmall>
        <div className={column}>
          <Box>{renderReadOnlyFields(statusFields)}</Box>
          {manifestContent !== undefined && renderJsonBox('Content', manifestContent)}
        </div>
      </section>

      <section>
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          Configuration
        </HeadingSmall>
        <div className={column}>
          <Box>{renderReadOnlyFields(configurationFields)}</Box>
          {input !== undefined && renderJsonBox('Input', input)}
        </div>
      </section>
    </div>
  );
}

function findStepByName(
  steps: PipelineRunStepInfo[] | undefined,
  name: string
): PipelineRunStepInfo | undefined {
  return steps?.find((step) => step.name === name);
}

/**
 * The run's input parameters as plain JSON, or undefined when the run has none.
 * Accepts both a raw protobuf Struct and an already-decoded object.
 */
function decodeRunInput(input: unknown): unknown {
  if (input == null) {
    return undefined;
  }
  const decoded = isStruct(input) ? decodeStruct(input) : input;
  if (typeof decoded === 'object' && decoded !== null && Object.keys(decoded).length === 0) {
    return undefined;
  }
  return decoded;
}

/**
 * Unix seconds the run executed for: the logical time stamped by the trigger workflow
 * for scheduled runs, else the time the run object was created. Scheduled runs are
 * usually created moments before their slot, but backfills can be created long after.
 */
function getExecutionTimestampSeconds(run: PipelineRunSummary | undefined): string | undefined {
  const label = run?.metadata?.labels?.[EXECUTION_TIMESTAMP_LABEL];
  if (label) {
    if (!isNaN(Number(label))) {
      return label;
    }
    // Older writers recorded the label as an RFC 3339 string.
    const parsed = Date.parse(label);
    if (!isNaN(parsed)) {
      return String(Math.floor(parsed / 1000));
    }
  }
  return run?.metadata?.creationTimestamp?.seconds;
}

/**
 * Elapsed time from run creation to its last finished step, or to now while
 * the run is still executing. The run-level `status.endTime` proto field is
 * never populated, so the end bound comes from step timings instead.
 */
function formatRunDuration(run: PipelineRunSummary | undefined): string | null {
  const startSeconds = Number(run?.metadata?.creationTimestamp?.seconds);
  // cast: the API returns the state as a bare number; it always holds a PipelineRunState value
  const state = run?.status?.state as PipelineRunState | undefined;
  if (isNaN(startSeconds) || state === undefined) {
    return null;
  }

  if (TERMINAL_RUN_STATES.has(state)) {
    const stepEndTimes = (run?.status?.steps ?? [])
      .map((step) => Number(step.endTime?.seconds))
      .filter((seconds) => !isNaN(seconds));
    if (stepEndTimes.length === 0) {
      return null;
    }
    return formatDurationSeconds(Math.max(...stepEndTimes) - startSeconds);
  }

  if (state === PipelineRunState.RUNNING) {
    return formatDurationSeconds(Math.floor(Date.now() / 1000) - startSeconds);
  }

  // Queued or pending — nothing has run yet.
  return null;
}

function formatDurationSeconds(totalSeconds: number): string {
  const seconds = Math.max(0, Math.round(totalSeconds));
  const parts: string[] = [];
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;

  if (hours > 0) {
    parts.push(`${hours} ${hours === 1 ? 'hour' : 'hours'}`);
  }
  if (minutes > 0) {
    parts.push(`${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`);
  }
  if (remainder > 0 || parts.length === 0) {
    parts.push(`${remainder} ${remainder === 1 ? 'second' : 'seconds'}`);
  }

  return parts.join(' ');
}
