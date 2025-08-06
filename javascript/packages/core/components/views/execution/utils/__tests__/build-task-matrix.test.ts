import { TASK_STATE } from '../../constants';
import { createTask } from '../__fixtures__/build-task-matrix-fixtures';
import { buildTaskMatrix } from '../build-task-matrix';

describe('buildTaskMatrix', () => {
  it('should return empty array for empty task list', () => {
    const result = buildTaskMatrix([]);
    expect(result).toEqual([]);
  });

  it('should return single row for tasks without subtasks', () => {
    const tasks = [
      createTask('Task 1', TASK_STATE.SUCCESS),
      createTask('Task 2', TASK_STATE.RUNNING, true),
      createTask('Task 3', TASK_STATE.PENDING),
    ];

    const result = buildTaskMatrix(tasks);

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual({
      parent: undefined,
      taskList: tasks,
    });
  });

  it('should create matrix rows for focused task with subtasks', () => {
    const subTasks = [
      createTask('Sub Task 1', TASK_STATE.SUCCESS),
      createTask('Sub Task 2', TASK_STATE.RUNNING, true),
    ];

    const tasks = [
      createTask('Task 1', TASK_STATE.SUCCESS),
      createTask('Task 2', TASK_STATE.RUNNING, true, subTasks),
      createTask('Task 3', TASK_STATE.PENDING),
    ];

    const result = buildTaskMatrix(tasks);

    expect(result).toHaveLength(2);
    expect(result[0]).toEqual({
      parent: undefined,
      taskList: tasks,
    });
    expect(result[1]).toEqual({
      parent: tasks[1], // The focused task with subtasks
      taskList: subTasks,
    });
  });

  it('should handle nested subtasks recursively', () => {
    const deepSubTasks = [
      createTask('Deep Sub 1', TASK_STATE.SUCCESS),
      createTask('Deep Sub 2', TASK_STATE.RUNNING, true),
    ];

    const subTasks = [
      createTask('Sub Task 1', TASK_STATE.SUCCESS),
      createTask('Sub Task 2', TASK_STATE.RUNNING, true, deepSubTasks),
    ];

    const tasks = [
      createTask('Task 1', TASK_STATE.SUCCESS),
      createTask('Task 2', TASK_STATE.RUNNING, true, subTasks),
    ];

    const result = buildTaskMatrix(tasks);

    expect(result).toHaveLength(3);
    expect(result[0]).toEqual({
      parent: undefined,
      taskList: tasks,
    });
    expect(result[1]).toEqual({
      parent: tasks[1],
      taskList: subTasks,
    });
    expect(result[2]).toEqual({
      parent: subTasks[1],
      taskList: deepSubTasks,
    });
  });

  it('should use last task when no task is focused', () => {
    const subTasks = [
      createTask('Sub Task 1', TASK_STATE.SUCCESS),
      createTask('Sub Task 2', TASK_STATE.PENDING),
    ];

    const tasks = [
      createTask('Task 1', TASK_STATE.SUCCESS),
      createTask('Task 2', TASK_STATE.PENDING, false, subTasks), // Not focused but has subtasks
    ];

    const result = buildTaskMatrix(tasks);

    expect(result).toHaveLength(2);
    expect(result[0]).toEqual({
      parent: undefined,
      taskList: tasks,
    });
    expect(result[1]).toEqual({
      parent: tasks[1], // Last task is used
      taskList: subTasks,
    });
  });

  it('should handle mixed scenarios with some tasks having subtasks', () => {
    const subTasks = [
      createTask('Sub Task A', TASK_STATE.SUCCESS),
      createTask('Sub Task B', TASK_STATE.RUNNING, true),
    ];

    const tasks = [
      createTask('Task 1', TASK_STATE.SUCCESS), // No subtasks
      createTask('Task 2', TASK_STATE.RUNNING, true, subTasks), // Has subtasks and focused
      createTask('Task 3', TASK_STATE.PENDING), // No subtasks
    ];

    const result = buildTaskMatrix(tasks);

    expect(result).toHaveLength(2);
    expect(result[0]).toEqual({
      parent: undefined,
      taskList: tasks,
    });
    expect(result[1]).toEqual({
      parent: tasks[1],
      taskList: subTasks,
    });
  });

  it('should handle complex hierarchy with multiple levels', () => {
    const level3Tasks = [
      createTask('Level 3 Task 1', TASK_STATE.SUCCESS),
      createTask('Level 3 Task 2', TASK_STATE.RUNNING, true),
    ];

    const level2Tasks = [
      createTask('Level 2 Task 1', TASK_STATE.SUCCESS),
      createTask('Level 2 Task 2', TASK_STATE.RUNNING, true, level3Tasks),
    ];

    const level1Tasks = [
      createTask('Level 1 Task 1', TASK_STATE.SUCCESS),
      createTask('Level 1 Task 2', TASK_STATE.RUNNING, true, level2Tasks),
    ];

    const result = buildTaskMatrix(level1Tasks);

    expect(result).toHaveLength(3);
    expect(result[0].taskList).toBe(level1Tasks);
    expect(result[1].taskList).toBe(level2Tasks);
    expect(result[1].parent).toBe(level1Tasks[1]);
    expect(result[2].taskList).toBe(level3Tasks);
    expect(result[2].parent).toBe(level2Tasks[1]);
  });
});
