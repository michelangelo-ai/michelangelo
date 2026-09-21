import { CellType } from '#core/components/cell/constants';

import type { Cell } from '#core/components/cell/types';

/**
 * Mirrors the generated proto TriggerRunState enum (trigger_run.proto). Colocated here until
 * core has access to the shared generated package — swapping the import path is the only change
 * needed then, since usage sites reference `TriggerRunState.RUNNING` etc.
 */
export const TriggerRunState = {
  INVALID: 'TRIGGER_RUN_STATE_INVALID',
  RUNNING: 'TRIGGER_RUN_STATE_RUNNING',
  KILLED: 'TRIGGER_RUN_STATE_KILLED',
  FAILED: 'TRIGGER_RUN_STATE_FAILED',
  SUCCEEDED: 'TRIGGER_RUN_STATE_SUCCEEDED',
  PENDING_KILL: 'TRIGGER_RUN_STATE_PENDING_KILL',
  PAUSED: 'TRIGGER_RUN_STATE_PAUSED',
} as const;

export const TRIGGER_STATE_CELL_CONFIG: Cell = {
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    [TriggerRunState.INVALID]: 'Queued',
    [TriggerRunState.RUNNING]: 'Running',
    [TriggerRunState.KILLED]: 'Killed',
    [TriggerRunState.FAILED]: 'Failed',
    [TriggerRunState.SUCCEEDED]: 'Succeeded',
    [TriggerRunState.PENDING_KILL]: 'Pending Kill',
  },
  stateColorMap: {
    [TriggerRunState.INVALID]: 'gray',
    [TriggerRunState.RUNNING]: 'blue',
    [TriggerRunState.KILLED]: 'gray',
    [TriggerRunState.FAILED]: 'red',
    [TriggerRunState.SUCCEEDED]: 'green',
    [TriggerRunState.PAUSED]: 'yellow',
  },
};

export const TRIGGER_PIPELINE_CELL_CONFIG: Cell = {
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
