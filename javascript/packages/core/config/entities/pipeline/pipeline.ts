import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';
import { RunTriggerForm } from './run-trigger-form';
import { isPipelineRevision } from './types';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { Pipeline } from './types';

/**
 * A record without a manifest (still loading, or a pipeline registered without one) means
 * "unknown", not "no triggers" — fail open and let the dialog explain an empty trigger list.
 * `record` is a live Pipeline from the "Pipelines" list row or the wrapping Revision from the
 * (always-revisioned) pipeline detail page — the manifest reads from `spec.content` for the
 * latter.
 */
const hasNoTriggers = (record: unknown): boolean => {
  // cast: record is unknown from the action predicate context; a live Pipeline when it isn't a
  // PipelineRevision; see #1425
  const pipeline = record as Pipeline;
  const manifest = isPipelineRevision(record)
    ? record.spec.content?.spec?.manifest
    : pipeline.spec?.manifest;
  return !!manifest && Object.keys(manifest.triggerMap ?? {}).length === 0;
};

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'pipelines',
  service: 'pipeline',
  state: 'active',
  revisioned: true,
  views: [PIPELINE_LIST_CONFIG, PIPELINE_DETAIL_CONFIG],
  actions: [
    {
      display: { label: 'Run', icon: 'playerPlay' },
      hierarchy: ActionHierarchy.PRIMARY,
      modal: { type: 'custom', component: CreatePipelineRunForm },
    },
    {
      display: { label: 'Run trigger', icon: 'calendarRepeat' },
      hierarchy: ActionHierarchy.SECONDARY,
      disabled: [
        {
          condition: interpolate(({ data }) => hasNoTriggers(data)),
          message: 'No triggers defined for this pipeline',
        },
      ],
      modal: { type: 'custom', component: RunTriggerForm },
    },
    {
      display: { label: 'Delete', icon: 'trashCan' },
      hierarchy: ActionHierarchy.TERTIARY,
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'DeletePipeline',
          // The record on the pipeline detail page is the viewed PipelineRevision, not the
          // Pipeline — deleteCrd (packages/rpc/handlers.ts) only reads `metadata.name`/
          // `metadata.namespace`, so retargeting `metadata` at `spec.baseResource` (the
          // Revision's pointer to the Pipeline it snapshots) is enough to delete the right
          // resource. A no-op on the list page, where `record` is already a Pipeline with no
          // `spec.baseResource`.
          middleware: {
            operations: [
              { source: 'spec.baseResource', destination: 'metadata', transformation: (v) => v },
            ],
          },
          successOperations: [
            { type: 'invalidate', targets: ['ListPipeline'] },
            { type: 'route', route: '/${studio.projectId}/${studio.phase}/pipelines' },
          ],
        },
      },
      modal: {
        type: 'confirm',
        header: { title: 'Delete Pipeline' },
        body: interpolate(({ data }) => {
          // cast: data is unknown from interpolation context; a live Pipeline from list rows,
          // or the wrapping PipelineRevision on the always-revisioned detail page; see #1425
          const pipeline = data as Pipeline;
          const name = isPipelineRevision(data)
            ? data.spec.baseResource.name
            : pipeline.metadata.name;
          return `Delete pipeline **${name}**? This action cannot be undone.`;
        }),
        button: { label: 'Delete' },
        destructive: true,
      },
    },
  ],
};
