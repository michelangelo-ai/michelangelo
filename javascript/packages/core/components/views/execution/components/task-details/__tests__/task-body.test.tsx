import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { createTask } from '../__fixtures__/task-details-fixtures';
import { TaskBody } from '../task-body';

import type { Task } from '#core/components/views/execution/types';
import type { TaskBodySchema } from '../renderers/types';

describe('TaskBody', () => {
  it('should render task overview and details when subtasks exist', () => {
    const taskWithSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [
        createTask({ name: 'Child Task 1' }),
        createTask({ name: 'Child Task 2' }),
        createTask({ name: 'Child Task 3' }),
      ],
    });

    render(<TaskBody task={taskWithSubtasks} />, buildWrapper([getRouterWrapper()]));

    expect(screen.getAllByText('Child Task 1')).toHaveLength(2);
    expect(screen.getAllByText('Child Task 2')).toHaveLength(2);
    expect(screen.getAllByText('Child Task 3')).toHaveLength(2);
  });

  it('should handle single subtask correctly', () => {
    const taskWithOneSubtask = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Only Child' })],
    });

    render(<TaskBody task={taskWithOneSubtask} />, buildWrapper([getRouterWrapper()]));

    expect(screen.getAllByText('Only Child')).toHaveLength(2);
  });

  it('should handle tasks with different states in subtasks', () => {
    const taskWithMixedSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [
        createTask({ name: 'Success Task', state: TASK_STATE.SUCCESS }),
        createTask({ name: 'Running Task', state: TASK_STATE.RUNNING }),
        createTask({ name: 'Error Task', state: TASK_STATE.ERROR }),
      ],
    });

    render(<TaskBody task={taskWithMixedSubtasks} />, buildWrapper([getRouterWrapper()]));

    expect(screen.getAllByText('Success Task')).toHaveLength(2);
    expect(screen.getAllByText('Running Task')).toHaveLength(2);
    expect(screen.getAllByText('Error Task')).toHaveLength(2);
  });

  it('should render body schema when no subtasks exist', () => {
    const leafTask = createTask({
      name: 'Leaf Task',
      record: {
        displayName: 'Leaf Task',
        output: { result: 'success' },
      },
    });

    const bodySchema = [
      {
        type: 'struct' as const,
        label: 'Task Output',
        accessor: 'output',
      },
    ];

    render(
      <TaskBody task={leafTask} bodySchema={bodySchema} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByText('Task Output')).toBeInTheDocument();
  });

  it('should prioritize subtasks over body schema', () => {
    const taskWithBoth = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Child Task' })],
      record: {
        output: { result: 'success' },
      },
    });

    const bodySchema = [
      {
        type: 'struct' as const,
        label: 'Should Not Render',
        accessor: 'output',
      },
    ];

    render(
      <TaskBody task={taskWithBoth} bodySchema={bodySchema} />,
      buildWrapper([getRouterWrapper()])
    );

    // Should render subtask, not body schema
    expect(screen.getAllByText('Child Task')).toHaveLength(2);
    expect(screen.queryByText('Should Not Render')).not.toBeInTheDocument();
  });

  it('should render textarea renderer for textarea type', () => {
    const taskWithTextarea = createTask({
      name: 'Log Task',
      record: {
        errorLog: 'Pipeline failed at step 3',
      },
    });

    const bodySchema = [
      {
        type: 'textarea' as const,
        label: 'Error Log',
        accessor: 'errorLog',
        markdown: false,
      },
    ];

    render(
      <TaskBody task={taskWithTextarea} bodySchema={bodySchema} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByText('Error Log')).toBeInTheDocument();
    expect(screen.getByText('Pipeline failed at step 3')).toBeInTheDocument();
  });

  it('should render metadata renderer for metadata type', async () => {
    const user = userEvent.setup();
    const taskWithMetadata = createTask({
      name: 'Task with Metadata',
      record: {
        metadata: {
          status: 'Success',
          duration: '5m 30s',
        },
      },
    });

    const bodySchema = [
      {
        type: 'metadata' as const,
        label: 'Task Metadata',
        cells: [
          {
            id: 'status',
            label: 'Status',
            type: CellType.TEXT,
            accessor: 'metadata.status',
          },
          {
            id: 'duration',
            label: 'Duration',
            type: CellType.TEXT,
            accessor: 'metadata.duration',
          },
        ],
      },
    ];

    render(
      <TaskBody task={taskWithMetadata} bodySchema={bodySchema} />,
      buildWrapper([getRouterWrapper()])
    );

    const accordionButton = screen.getByRole('button', { name: /Task Metadata/ });
    await user.click(accordionButton);

    expect(screen.getByText('Success')).toBeInTheDocument();
    expect(screen.getByText('5m 30s')).toBeInTheDocument();
  });

  it('should handle unknown schema types gracefully', () => {
    const taskWithUnknownSchema = createTask({
      name: 'Task with Unknown Schema',
      record: { data: 'some data' },
    });

    const bodySchema = [
      {
        type: 'unknown-type',
        label: 'Unknown',
        accessor: 'data',
      } as unknown as TaskBodySchema,
    ];

    render(
      <TaskBody task={taskWithUnknownSchema} bodySchema={bodySchema} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.queryByText('Unknown')).not.toBeInTheDocument();
  });

  it('should pass the current task as parent to TaskListRenderer when rendering subtasks', () => {
    const ParentLabelRenderer = ({ parent }: { parent?: Task }) => (
      <div>subtasks of {parent?.name ?? 'unknown'}</div>
    );

    const taskWithSubtasks = createTask({
      name: 'Execute Workflow',
      subTasks: [createTask({ name: 'Child 1' }), createTask({ name: 'Child 2' })],
    });

    render(
      <TaskBody
        task={taskWithSubtasks}
        overrides={{ TaskListRenderer: { component: ParentLabelRenderer } }}
      />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByText('subtasks of Execute Workflow')).toBeInTheDocument();
  });
});
