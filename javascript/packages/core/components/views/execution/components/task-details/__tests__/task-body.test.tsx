import { render, screen } from '@testing-library/react';

import { TASK_STATE } from '#core/components/views/execution/constants';
import { createTask } from '../__fixtures__/task-details-fixtures';
import { TaskBody } from '../task-body';

describe('TaskBody', () => {
  it('should render TaskNodeList when subtasks exist', () => {
    const taskWithSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [
        createTask({ name: 'Child Task 1' }),
        createTask({ name: 'Child Task 2' }),
        createTask({ name: 'Child Task 3' }),
      ],
    });

    render(<TaskBody task={taskWithSubtasks} />);

    expect(screen.getByText('Child Task 1')).toBeInTheDocument();
    expect(screen.getByText('Child Task 2')).toBeInTheDocument();
    expect(screen.getByText('Child Task 3')).toBeInTheDocument();
  });

  it('should handle single subtask correctly', () => {
    const taskWithOneSubtask = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Only Child' })],
    });

    render(<TaskBody task={taskWithOneSubtask} />);

    expect(screen.getByText('Only Child')).toBeInTheDocument();
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

    render(<TaskBody task={taskWithMixedSubtasks} />);

    expect(screen.getByText('Success Task')).toBeInTheDocument();
    expect(screen.getByText('Running Task')).toBeInTheDocument();
    expect(screen.getByText('Error Task')).toBeInTheDocument();
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
        type: 'struct',
        label: 'Task Output',
        accessor: 'output',
      },
    ];

    render(<TaskBody task={leafTask} bodySchema={bodySchema} />);

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
        type: 'struct',
        label: 'Should Not Render',
        accessor: 'output',
      },
    ];

    render(<TaskBody task={taskWithBoth} bodySchema={bodySchema} />);

    // Should render subtask, not body schema
    expect(screen.getByText('Child Task')).toBeInTheDocument();
    expect(screen.queryByText('Should Not Render')).not.toBeInTheDocument();
  });
});
