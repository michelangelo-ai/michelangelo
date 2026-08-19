import { CellType } from '#core/components/cell/constants';
import {
  SHARED_RUN_CELL_CONFIG,
  TRIGGERED_BY_CELL_CONFIG,
  TRIGGERED_BY_LABEL,
} from '#core/config/entities/run/shared';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';

import type { DetailViewConfig } from '#core/components/views/types';

/**
 * Every row on this tab is already scoped to the trigger being viewed, so the
 * "Triggered by" column would repeat the page's own name and link back to itself.
 */
const RUN_CELLS_EXCLUDING_TRIGGER = SHARED_RUN_CELL_CONFIG.filter(
  (cell) => cell !== TRIGGERED_BY_CELL_CONFIG
);

export const TRIGGER_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'spec.actor.name', label: 'Started by', type: CellType.TEXT },
    TRIGGER_PIPELINE_CELL_CONFIG,
    TRIGGER_STATE_CELL_CONFIG,
  ],
  pages: [
    {
      id: 'runs',
      label: 'Recent Runs',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'pipelineRun',
        serviceOptions: {
          listOptions: {
            // `\${...}` escapes the interpolation token so it survives this template
            // literal and is resolved later against the page data.
            labelSelector: `${TRIGGERED_BY_LABEL}=\${page.metadata.name}`,
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
          ...RUN_CELLS_EXCLUDING_TRIGGER,
        ],
      },
    },
  ],
};
