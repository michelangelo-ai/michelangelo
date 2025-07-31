import { TASK_STATE } from '../../constants';
import { createSchema, createTask } from '../__fixtures__/build-task-list-fixtures';
import { buildTaskList } from '../build-task-list';

describe('buildTaskList', () => {
  const mockSchema = createSchema();

  it('should build task list from simple data structure', () => {
    const data = {
      steps: [createTask('Task 1', 'SUCCEEDED'), createTask('Task 2', 'RUNNING')],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({
      name: 'Task 1',
      state: TASK_STATE.SUCCESS,
      active: false,
    });
    expect(result[1]).toMatchObject({
      name: 'Task 2',
      state: TASK_STATE.RUNNING,
      active: true, // First non-success/non-skipped task is active
    });
    expect(result[0].record).toEqual(data.steps[0]);
  });

  it('should handle hierarchical tasks with subSteps', () => {
    const data = {
      steps: [
        {
          displayName: 'Parent Task',
          state: 'SUCCEEDED',
          subSteps: [
            {
              displayName: 'Child Task 1',
              state: 'SUCCEEDED',
              subSteps: [],
            },
            {
              displayName: 'Child Task 2',
              state: 'RUNNING',
              subSteps: [],
            },
          ],
        },
      ],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result).toHaveLength(1);
    expect(result[0].subTasks).toHaveLength(2);
    expect(result[0].subTasks[0]).toEqual({
      name: 'Child Task 1',
      state: TASK_STATE.SUCCESS,
      subTasks: [],
      record: data.steps[0].subSteps[0],
      active: false,
    });
    expect(result[0].subTasks[1]).toEqual({
      name: 'Child Task 2',
      state: TASK_STATE.RUNNING,
      subTasks: [],
      record: data.steps[0].subSteps[1],
      active: true,
    });
  });

  it('should handle function accessor for task heading', () => {
    const schemaWithFunctionAccessor = createSchema({
      tasks: {
        accessor: (data: { steps: object[] }) => data.steps,
        header: {
          heading: (record: { name: string }) => `Custom: ${record.name}`,
        },
      },
    });

    const data = {
      steps: [createTask('build-task', 'SUCCEEDED', [], { name: 'build-task' })],
    };

    expect(buildTaskList(schemaWithFunctionAccessor, data)[0].name).toBe('Custom: build-task');
  });

  it('should fallback to "name" field when heading accessor returns nothing', () => {
    const data = {
      steps: [
        {
          name: 'fallback-task',
          state: 'SUCCEEDED',
          subSteps: [],
        },
      ],
    };

    expect(buildTaskList(mockSchema, data)[0].name).toBe('fallback-task');
  });

  it('should handle missing subTasksAccessor gracefully', () => {
    const schemaWithoutSubTasks = createSchema({
      tasks: {
        // @ts-expect-error null ensures merging overrides default subTasksAccessor provided by createSchema
        subTasksAccessor: null,
      },
    });

    const data = {
      steps: [
        {
          displayName: 'Task 1',
          state: 'SUCCEEDED',
          subSteps: [{ displayName: 'Should be ignored' }],
        },
      ],
    };

    expect(buildTaskList(schemaWithoutSubTasks, data)[0].subTasks).toEqual([]);
  });

  it('should determine active task correctly - first non-success/non-skipped', () => {
    const data = {
      steps: [
        { displayName: 'Task 1', state: 'SUCCEEDED', subSteps: [] },
        { displayName: 'Task 2', state: 'SUCCEEDED', subSteps: [] },
        { displayName: 'Task 3', state: 'RUNNING', subSteps: [] },
        { displayName: 'Task 4', state: 'PENDING', subSteps: [] },
      ],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result[0].active).toBe(false); // Succeeded
    expect(result[1].active).toBe(false); // Succeeded
    expect(result[2].active).toBe(true); // First non-success
    expect(result[3].active).toBe(false); // Not the first non-success
  });

  it('should mark last task as active when all others are success/skipped', () => {
    const schemaWithSkipped = createSchema({
      tasks: {
        stateBuilder: (record: { state: string }) => {
          switch (record.state) {
            case 'SUCCEEDED':
              return TASK_STATE.SUCCESS;
            case 'SKIPPED':
              return TASK_STATE.SKIPPED;
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    });

    const data = {
      steps: [
        { displayName: 'Task 1', state: 'SUCCEEDED', subSteps: [] },
        { displayName: 'Task 2', state: 'SKIPPED', subSteps: [] },
        { displayName: 'Task 3', state: 'SUCCEEDED', subSteps: [] },
      ],
    };

    const result = buildTaskList(schemaWithSkipped, data);

    expect(result[0].active).toBe(false);
    expect(result[1].active).toBe(false);
    expect(result[2].active).toBe(true); // Last task when all others are success/skipped
  });

  it('should handle empty data structure', () => {
    const data = {
      steps: [],
    };

    expect(buildTaskList(mockSchema, data)).toEqual([]);
  });

  it('should handle missing nested data gracefully', () => {
    const data = {};

    expect(buildTaskList(mockSchema, data)).toEqual([]);
  });

  it('should preserve original record data for each task', () => {
    const data = {
      steps: [
        {
          displayName: 'Task 1',
          state: 'SUCCEEDED',
          subSteps: [],
          customField: 'custom-value',
          metadata: { key: 'value' },
        },
      ],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result[0].record).toEqual(data.steps[0]);
  });
});
