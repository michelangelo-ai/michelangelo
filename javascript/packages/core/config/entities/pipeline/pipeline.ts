import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';
import { CreatePipelineRunForm } from './create-pipeline-run-form';
import { PIPELINE_DETAIL_CONFIG } from './detail';
import { PIPELINE_LIST_CONFIG } from './list';

import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { Pipeline } from './types';

export const PIPELINE_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'pipelines',
  name: 'Pipelines',
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
      display: { label: 'Delete', icon: 'trashCan' },
      hierarchy: ActionHierarchy.TERTIARY,
      operation: {
        type: 'mutation',
        mutation: {
          mutationName: 'DeletePipeline',
          // DeletePipelineRequest is flat ({ name, namespace }); the record is a full
          // Pipeline (metadata/spec/status/typeMeta). Reshape to the request shape —
          // the backend rejects unknown fields.
          middleware: {
            operations: [
              { source: 'metadata.name', destination: 'name', transformation: (value) => value },
              {
                source: 'metadata.namespace',
                destination: 'namespace',
                transformation: (value) => value,
              },
              { destination: 'metadata', transformation: 'unset' },
              { destination: 'spec', transformation: 'unset' },
              { destination: 'status', transformation: 'unset' },
              { destination: 'typeMeta', transformation: 'unset' },
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
