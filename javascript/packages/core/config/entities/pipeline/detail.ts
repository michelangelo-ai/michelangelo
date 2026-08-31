import { CellType } from '#core/components/cell/constants';
import { SHARED_RUN_CELL_CONFIG } from '#core/config/entities/run/shared';
import {
  CRITERION_OPERATOR_EQUAL,
  PIPELINE_RUN_PIPELINE_NAME_FIELD,
  PIPELINE_STATE_CELL,
  PIPELINE_TYPE_CELL,
} from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

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
              criterion: [
                {
                  fieldName: PIPELINE_RUN_PIPELINE_NAME_FIELD,
                  operator: CRITERION_OPERATOR_EQUAL,
                  matchValue: '${page.metadata.name}',
                },
              ],
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
  ],
};
