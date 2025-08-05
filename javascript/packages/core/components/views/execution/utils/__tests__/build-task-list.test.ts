import {
  buildExecutionSchemaFactory,
  buildTaskStepFactory,
} from '#core/components/views/execution/__fixtures__/execution-schema-factory';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { buildTaskList } from '../build-task-list';

describe('buildTaskList', () => {
  const buildSchema = buildExecutionSchemaFactory();
  const buildTask = buildTaskStepFactory();
  const mockSchema = buildSchema({ tasks: { accessor: 'steps' } });

  it('should build task list from simple data structure', () => {
    const data = {
      steps: [
        buildTask({ displayName: 'Task 1', state: 'SUCCEEDED' }),
        buildTask({ displayName: 'Task 2', state: 'RUNNING' }),
      ],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result).toHaveLength(2);
    expect(result[0]).toMatchObject({
      name: 'Task 1',
      state: TASK_STATE.SUCCESS,
      focused: false,
    });
    expect(result[1]).toMatchObject({
      name: 'Task 2',
      state: TASK_STATE.RUNNING,
      focused: true, // First non-success/non-skipped task is focused
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
      focused: false,
    });
    expect(result[0].subTasks[1]).toEqual({
      name: 'Child Task 2',
      state: TASK_STATE.RUNNING,
      subTasks: [],
      record: data.steps[0].subSteps[1],
      focused: true,
    });
  });

  it('should handle function accessor for task heading', () => {
    const schemaWithFunctionAccessor = buildSchema({
      tasks: {
        accessor: (data: { steps: object[] }) => data.steps,
        header: {
          heading: (record: { name: string }) => `Custom: ${record.name}`,
        },
      },
    });

    const data = {
      steps: [
        buildTask({
          displayName: 'build-task',
          state: 'SUCCEEDED',
          subSteps: [],
          name: 'build-task',
        }),
      ],
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
    const schemaWithoutSubTasks = buildSchema({
      tasks: {
        accessor: 'steps',
        // @ts-expect-error null ensures merging overrides default subTasksAccessor provided by buildSchema
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

  it('should determine focused task correctly - first non-success/non-skipped', () => {
    const data = {
      steps: [
        { displayName: 'Task 1', state: 'SUCCEEDED', subSteps: [] },
        { displayName: 'Task 2', state: 'SUCCEEDED', subSteps: [] },
        { displayName: 'Task 3', state: 'RUNNING', subSteps: [] },
        { displayName: 'Task 4', state: 'PENDING', subSteps: [] },
      ],
    };

    const result = buildTaskList(mockSchema, data);

    expect(result[0].focused).toBe(false); // Succeeded
    expect(result[1].focused).toBe(false); // Succeeded
    expect(result[2].focused).toBe(true); // First non-success
    expect(result[3].focused).toBe(false); // Not the first non-success
  });

  it('should mark last task as focused when all others are success/skipped', () => {
    const schemaWithSkipped = buildSchema({
      tasks: {
        accessor: 'steps',
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

    expect(result[0].focused).toBe(false);
    expect(result[1].focused).toBe(false);
    expect(result[2].focused).toBe(true); // Last task when all others are success/skipped
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
