import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';
import { RunTriggerForm } from './run-trigger-form';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { Pipeline } from './types';

/**
 * A record without a manifest (still loading, or a pipeline registered without one) means
 * "unknown", not "no triggers" — fail open and let the dialog explain an empty trigger list.
 */
const hasNoTriggers = (record: unknown): boolean => {
  // cast: record is unknown from the action predicate context; always Pipeline in this entity
  // config; see #1425
  const manifest = (record as Pipeline).spec?.manifest;
  return !!manifest && Object.keys(manifest.triggerMap ?? {}).length === 0;
};

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'pipelines',
  service: 'pipeline',
  state: 'active',
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
          successOperations: [
            { type: 'invalidate', targets: ['ListPipeline'] },
            { type: 'route', route: '/${studio.projectId}/${studio.phase}/pipelines' },
          ],
        },
      },
      modal: {
        type: 'confirm',
        header: { title: 'Delete Pipeline' },
        body: interpolate(
          ({ data }) =>
            // cast: data is unknown from interpolation context; always Pipeline in this entity
            // config; see #1425
            `Delete pipeline **${(data as Pipeline).metadata.name}**? This action cannot be undone.`
        ),
        button: { label: 'Delete' },
        destructive: true,
      },
    },
  ],
};
