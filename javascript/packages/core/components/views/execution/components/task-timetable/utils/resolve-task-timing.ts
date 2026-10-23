import { isNil } from 'lodash';

import { toEpochSeconds } from '#core/utils/time-utils';

import type { Task } from '../../../types';
import type { TaskTiming } from './types';

/**
 * Reads the `startTime` and `endTime` of a task's raw record as epoch seconds.
 * Bounds that are absent or unparseable are left undefined.
 */
export function resolveTaskTiming<TTaskRecord extends object>(task: Task<TTaskRecord>): TaskTiming {
  // cast: the record shape is schema-defined; execution schemas expose startTime/endTime strings
  const { startTime, endTime } = task.record as { startTime?: string; endTime?: string };

  return { start: toOptionalEpochSeconds(startTime), end: toOptionalEpochSeconds(endTime) };
}

function toOptionalEpochSeconds(timestamp?: string): number | undefined {
  if (isNil(timestamp) || timestamp === '') {
    return undefined;
  }

  const seconds = toEpochSeconds(timestamp);
  return isNaN(seconds) ? undefined : seconds;
}
