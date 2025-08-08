import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { Task } from '#core/components/views/execution/types';
import { determineExecutionState } from '../determine-execution-state';

describe('determineExecutionState', () => {
  it('should return PENDING for empty task list', () => {
    expect(determineExecutionState([])).toBe(TASK_STATE.PENDING);
  });

  it('should return RUNNING when any task is running', () => {
    const tasks = [
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.RUNNING }),
      createTask({ state: TASK_STATE.PENDING }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.RUNNING);
  });

  it('should return RUNNING when multiple tasks are running', () => {
    const tasks = [
      createTask({ state: TASK_STATE.RUNNING }),
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.RUNNING }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.RUNNING);
  });

  it('should return ERROR when any task failed and none are running', () => {
    const tasks = [
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.ERROR }),
      createTask({ state: TASK_STATE.PENDING }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.ERROR);
  });

  it('should return ERROR when multiple tasks failed and none are running', () => {
    const tasks = [
      createTask({ state: TASK_STATE.ERROR }),
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.ERROR }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.ERROR);
  });

  it('should prioritize RUNNING over ERROR', () => {
    const tasks = [
      createTask({ state: TASK_STATE.ERROR }),
      createTask({ state: TASK_STATE.RUNNING }),
      createTask({ state: TASK_STATE.ERROR }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.RUNNING);
  });

  it('should return last task state when no running or error tasks', () => {
    const tasks = [
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.SKIPPED }),
      createTask({ state: TASK_STATE.SUCCESS }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.SUCCESS);
  });

  it('should return last task state for pending tasks', () => {
    const tasks = [
      createTask({ state: TASK_STATE.PENDING }),
      createTask({ state: TASK_STATE.PENDING }),
      createTask({ state: TASK_STATE.PENDING }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.PENDING);
  });

  it('should handle single task correctly', () => {
    expect(determineExecutionState([createTask({ state: TASK_STATE.SUCCESS })])).toBe(
      TASK_STATE.SUCCESS
    );
    expect(determineExecutionState([createTask({ state: TASK_STATE.ERROR })])).toBe(
      TASK_STATE.ERROR
    );
    expect(determineExecutionState([createTask({ state: TASK_STATE.RUNNING })])).toBe(
      TASK_STATE.RUNNING
    );
    expect(determineExecutionState([createTask({ state: TASK_STATE.PENDING })])).toBe(
      TASK_STATE.PENDING
    );
    expect(determineExecutionState([createTask({ state: TASK_STATE.SKIPPED })])).toBe(
      TASK_STATE.SKIPPED
    );
  });

  it('should handle mixed states with success at end', () => {
    const tasks = [
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.SKIPPED }),
      createTask({ state: TASK_STATE.SUCCESS }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.SUCCESS);
  });

  it('should handle mixed states with pending at end', () => {
    const tasks = [
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.SUCCESS }),
      createTask({ state: TASK_STATE.PENDING }),
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.PENDING);
  });

  it('should handle edge case where last task state is undefined', () => {
    const tasks = [
      {
        name: 'Test Task',
        state: undefined,
        subTasks: [],
        record: {},
        focused: false,
      } as unknown as Task,
    ];

    expect(determineExecutionState(tasks)).toBe(TASK_STATE.PENDING);
  });
});
