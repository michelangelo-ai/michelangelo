import { CellType } from '#core/components/cell/constants';
import {
  RUN_TRIGGERED_BY_COLUMN,
  SHARED_RUN_CELL_CONFIG,
  TRIGGERED_BY_LABEL,
} from '#core/config/entities/run/shared';
import { TRIGGER_PIPELINE_CELL_CONFIG, TRIGGER_STATE_CELL_CONFIG } from './shared';
import { TriggerInfoPage } from './trigger-info-page';

import type { DetailViewConfig } from '#core/components/views/types';
import type { TriggerRun } from './types';

/**
 * Every row on this tab is already scoped to the trigger being viewed, so the
 * "Triggered by" column would repeat the page's own name and link back to itself.
 */
const RUN_CELLS_EXCLUDING_TRIGGER = SHARED_RUN_CELL_CONFIG.filter(
  (cell) => cell !== RUN_TRIGGERED_BY_COLUMN
);

export const TRIGGER_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    TRIGGER_PIPELINE_CELL_CONFIG,
    { id: 'metadata.creationTimestamp.seconds', label: 'Creation time', type: CellType.DATE },
    {
      id: 'spec.trigger.triggerType.value.cron',
      label: 'Cron',
      // Hidden (not just blank) for a non-cron trigger, so it never shows alongside
      // "Interval seconds" for the same run.
      hideEmpty: true,
    },
    {
      id: 'interval-seconds',
      label: 'Interval seconds',
      hideEmpty: true,
      accessor: (record: unknown) => {
        // cast: accessor receives unknown data; narrowing to expected proto shape for
        // property access
        const trigger = (record as TriggerRun).spec?.trigger;
        if (trigger?.triggerType?.case !== 'intervalSchedule') {
          return undefined;
        }
        const seconds = trigger.triggerType.value?.interval?.seconds;
        return seconds === undefined ? undefined : Number(seconds);
      },
    },
    {
      id: 'batch-size',
      label: 'Batch size',
      hideEmpty: true,
      accessor: (record: unknown) => {
        // cast: accessor receives unknown data; narrowing to expected proto shape for
        // property access
        const trigger = (record as TriggerRun).spec?.trigger;
        return trigger?.maxConcurrency ? undefined : trigger?.batchPolicy?.batchSize;
      },
    },
    {
      id: 'wait-minutes',
      label: 'Wait minutes',
      hideEmpty: true,
      accessor: (record: unknown) => {
        // cast: accessor receives unknown data; narrowing to expected proto shape for
        // property access
        const trigger = (record as TriggerRun).spec?.trigger;
        if (trigger?.maxConcurrency || trigger?.batchPolicy?.waitSeconds === undefined) {
          return undefined;
        }
        return Math.round(Number(trigger.batchPolicy.waitSeconds) / 60);
      },
    },
    {
      id: 'spec.trigger.maxConcurrency',
      label: 'Max concurrency',
      hideEmpty: true,
    },
    {
      id: 'spec.startTimestamp.seconds',
      label: 'Start time',
      type: CellType.DATE,
      hideEmpty: true,
    },
    {
      id: 'spec.endTimestamp.seconds',
      label: 'End time',
      type: CellType.DATE,
      hideEmpty: true,
    },
    { id: 'spec.actor.name', label: 'Owner', type: CellType.TEXT },
    TRIGGER_STATE_CELL_CONFIG,
  ],
  pages: [
    {
      id: 'information',
      label: 'Information',
      type: 'custom',
      component: TriggerInfoPage,
    },
    {
      id: 'runs',
      label: 'Triggered Runs',
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
