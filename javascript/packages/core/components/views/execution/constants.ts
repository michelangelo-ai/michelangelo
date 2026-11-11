import type { StateStyleConfig, TaskState } from './types';

export const TASK_STATE = {
  ERROR: 'ERROR',
  PENDING: 'PENDING',
  RUNNING: 'RUNNING',
  SUCCESS: 'SUCCESS',
  SKIPPED: 'SKIPPED',
} as const;

export const STATE_TO_STYLE_MAP: Record<TaskState, StateStyleConfig> = {
  [TASK_STATE.ERROR]: {
    borderColorName: 'borderNegativeLight',
    backgroundColorName: 'backgroundNegativeLight',
  },
  [TASK_STATE.PENDING]: {
    borderColorName: 'contentInverseTertiary',
    colorName: 'contentInverseTertiary',
  },
  [TASK_STATE.SKIPPED]: {
    borderColorName: 'contentInverseTertiary',
    colorName: 'contentInverseTertiary',
  },
  [TASK_STATE.RUNNING]: {
    borderColorName: 'borderAccent',
    backgroundColorName: 'backgroundAccentLight',
  },
  [TASK_STATE.SUCCESS]: {
    borderColorName: 'borderPositive',
  },
} as const;

export const STATE_TO_ICON_MAP = {
  [TASK_STATE.SUCCESS]: { name: 'circleCheck', colorName: 'eatsGreen400' },
  [TASK_STATE.ERROR]: { name: 'circleX', colorName: 'contentNegative' },
  [TASK_STATE.PENDING]: { name: 'diamondEmpty', colorName: 'contentInverseTertiary' },
  [TASK_STATE.RUNNING]: { name: 'arrowCircular', colorName: 'contentAccent' },
  [TASK_STATE.SKIPPED]: { name: 'playerNext', colorName: 'contentNegative' },
} as const;

export const EXECUTION_VIEW = {
  FLOW: 'FLOW',
  TIMETABLE: 'TIMETABLE',
} as const;

/** Order in which the overview views are offered in the view switch. */
export const EXECUTION_VIEW_OPTIONS = [
  { view: EXECUTION_VIEW.FLOW, label: 'Flow' },
  { view: EXECUTION_VIEW.TIMETABLE, label: 'Timetable' },
] as const;
