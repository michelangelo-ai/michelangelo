import { CellType } from '#core/components/cell/constants';

import type { RowCell } from '#core/components/row/types';
import type { TriggerRun } from './types';

export const TRIGGER_STATE_CELL_CONFIG: RowCell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    0: 'Queued',
    1: 'Running',
    2: 'Killed',
    3: 'Failed',
    4: 'Succeeded',
    5: 'Pending Kill',
  },
  stateColorMap: {
    0: 'gray',
    1: 'blue',
    2: 'gray',
    3: 'red',
    4: 'green',
    6: 'yellow',
  },
};

export const TRIGGER_PIPELINE_CELL_CONFIG: RowCell = {
  id: 'spec.pipeline.name',
  label: 'Pipeline',
  items: [
    {
      id: 'spec.pipeline.name',
      type: CellType.TEXT,
    },
    {
      id: 'spec.revision.name',
      type: CellType.DESCRIPTION,
    },
  ],
};

export const TRIGGER_CRON_CELL_CONFIG: RowCell = {
  id: 'spec.trigger.triggerType.value.cron',
  label: 'Cron Schedule',
  type: CellType.TEXT,
  accessor: (row: TriggerRun) => {
    const triggerType = row.spec?.trigger?.triggerType;
    if (triggerType?.case === 'cronSchedule') {
      return triggerType.value.cron;
    }
    return undefined;
  },
  hideEmpty: true,
};

export const TRIGGER_INTERVAL_CELL_CONFIG: RowCell = {
  id: 'spec.trigger.triggerType.value.interval.seconds',
  label: 'Interval (seconds)',
  type: CellType.TEXT,
  accessor: (row: TriggerRun) => {
    const triggerType = row.spec?.trigger?.triggerType;
    if (triggerType?.case === 'intervalSchedule') {
      return triggerType.value.interval?.seconds;
    }
    return undefined;
  },
  hideEmpty: true,
};

export const TRIGGER_BATCH_SIZE_CELL_CONFIG: RowCell = {
  id: 'spec.trigger.batchPolicy.batchSize',
  label: 'Batch Size',
  type: CellType.TEXT,
  accessor: (row: TriggerRun) => {
    if (row.spec?.trigger?.maxConcurrency) return undefined;
    return row.spec?.trigger?.batchPolicy?.batchSize;
  },
  hideEmpty: true,
};

export const TRIGGER_WAIT_CELL_CONFIG: RowCell = {
  id: 'spec.trigger.batchPolicy.wait.seconds',
  label: 'Wait (seconds)',
  type: CellType.TEXT,
  accessor: (row: TriggerRun) => {
    if (row.spec?.trigger?.maxConcurrency) return undefined;
    return row.spec?.trigger?.batchPolicy?.wait?.seconds;
  },
  hideEmpty: true,
};

export const TRIGGER_MAX_CONCURRENCY_CELL_CONFIG: RowCell = {
  id: 'spec.trigger.maxConcurrency',
  label: 'Max Concurrency',
  type: CellType.TEXT,
  accessor: (row: TriggerRun) => row.spec?.trigger?.maxConcurrency,
  hideEmpty: true,
};

export const TRIGGER_START_TIME_CELL_CONFIG: RowCell = {
  id: 'spec.startTimestamp.seconds',
  label: 'Start Time',
  type: CellType.DATE,
  hideEmpty: true,
};

export const TRIGGER_END_TIME_CELL_CONFIG: RowCell = {
  id: 'spec.endTimestamp.seconds',
  label: 'End Time',
  type: CellType.DATE,
  hideEmpty: true,
};
