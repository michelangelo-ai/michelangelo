import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import { TRIGGER_STATE_CELL_CONFIG } from '#core/config/entities/trigger/shared';
import { interpolate } from '#core/interpolation/interpolate';
import { buildRevisionName } from '#core/utils/revision-utils';
import { formatTriggerSchedule } from './format-trigger-schedule';
import {
  CRITERION_OPERATOR_EQUAL,
  PIPELINE_RUN_PIPELINE_NAME_FIELD,
  PIPELINE_RUN_REVISION_NAME_FIELD,
  PIPELINE_STATE_CELL,
  PIPELINE_TYPE_CELL,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';
import type { TriggerRun } from '#core/config/entities/trigger/types';
import type { Pipeline } from './types';

export const PIPELINE_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT },
    PIPELINE_TYPE_CELL,
    { id: 'spec.commit.branch', label: 'Branch', type: CellType.TEXT },
    PIPELINE_STATE_CELL,
  ],
  pages: [
    {
      id: 'runs',
      label: 'Runs',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'pipelineRun',
        serviceOptions: {
          listOptionsExt: {
            operation: {
              criterion: interpolate(({ page, studio }) => {
                // cast: page is unknown from interpolation context; always Pipeline in this
                // entity config (a revision view passes the snapshot's content); see #1425
                const pipelineName = (page as Pipeline).metadata.name;
                return [
                  studio.revisionId
                    ? {
                        fieldName: PIPELINE_RUN_REVISION_NAME_FIELD,
                        operator: CRITERION_OPERATOR_EQUAL,
                        matchValue: buildRevisionName('pipeline', pipelineName, studio.revisionId),
                      }
                    : {
                        fieldName: PIPELINE_RUN_PIPELINE_NAME_FIELD,
                        operator: CRITERION_OPERATOR_EQUAL,
                        matchValue: pipelineName,
                      },
                ];
              }),
            },
          },
        },
      },
      tableConfig: {
        columns: [
          {
            id: 'metadata.name',
            label: 'Name',
            url: '/${studio.projectId}/${studio.phase}/runs/${row.metadata.name}',
          },
          ...SHARED_RUN_CELL_CONFIG,
        ],
      },
    },
    {
      id: 'triggers',
      label: 'Triggers',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'triggerRun',
        serviceOptions: {
          listOptions: {
            fieldSelector: 'pipeline_name=${page.metadata.name}',
          },
        },
      },
      tableConfig: {
        columns: [
          {
            id: 'metadata.name',
            label: 'Name',
            url: '/${studio.projectId}/${studio.phase}/triggers/${row.metadata.name}',
          },
          { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
          { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
          {
            id: 'schedule',
            label: 'Schedule',
            type: CellType.TEXT,
            // cast: accessor rows are untyped in table config; always a TriggerRun on this
            // tab's query; see #1425
            accessor: (row: unknown) => formatTriggerSchedule((row as TriggerRun).spec?.trigger),
          },
          TRIGGER_STATE_CELL_CONFIG,
        ],
      },
    },
  ],
};
