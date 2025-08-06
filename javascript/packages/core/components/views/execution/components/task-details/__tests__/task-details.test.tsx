import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { createTask } from '../__fixtures__/task-details-fixtures';
import { TaskDetails } from '../task-details';

describe('TaskDetails', () => {
  it('should render simple header when task has no subtasks and no bodySchema', () => {
    const leafTask = createTask({ name: 'Leaf Task' });

    render(<TaskDetails task={leafTask} />);

    expect(screen.getByText('Leaf Task')).toBeInTheDocument();

    expect(screen.queryByRole('button', { expanded: false })).not.toBeInTheDocument();
  });

  it('should render accordion when task has subtasks', () => {
    const taskWithSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Child Task 1' }), createTask({ name: 'Child Task 2' })],
    });

    render(<TaskDetails task={taskWithSubtasks} />);

    expect(screen.getByText('Parent Task')).toBeInTheDocument();

    const accordionPanel = screen.getByRole('button', { expanded: false });
    expect(accordionPanel).toBeInTheDocument();
  });

  it('should render accordion when task has bodySchema but no subtasks', () => {
    const leafTaskWithBodySchema = createTask({ name: 'Data Processing Task' });
    const bodySchema = [
      {
        type: 'struct',
        label: 'Input Parameters',
        accessor: 'input',
      },
    ];

    render(<TaskDetails task={leafTaskWithBodySchema} bodySchema={bodySchema} />);

    expect(screen.getByText('Data Processing Task')).toBeInTheDocument();

    const accordionPanel = screen.getByRole('button', { expanded: false });
    expect(accordionPanel).toBeInTheDocument();
  });

  it('should start accordion collapsed by default', () => {
    const taskWithSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Child Task' })],
    });

    render(<TaskDetails task={taskWithSubtasks} />);

    const accordionPanel = screen.getByRole('button', { expanded: false });
    expect(accordionPanel).toBeInTheDocument();

    expect(screen.queryByText('Child Task')).not.toBeInTheDocument();
  });

  it('should expand accordion to show subtasks when clicked', async () => {
    const user = userEvent.setup();
    const taskWithSubtasks = createTask({
      name: 'Parent Task',
      subTasks: [createTask({ name: 'Child Task 1' }), createTask({ name: 'Child Task 2' })],
    });

    render(<TaskDetails task={taskWithSubtasks} />);

    const accordionPanel = screen.getByRole('button', { expanded: false });
    await user.click(accordionPanel);

    expect(screen.getByText('Child Task 1')).toBeInTheDocument();
    expect(screen.getByText('Child Task 2')).toBeInTheDocument();
  });

  it('should handle tasks with empty subtasks array as leaf task', () => {
    const taskWithEmptySubtasks = createTask({ name: 'Task', subTasks: [] });

    render(<TaskDetails task={taskWithEmptySubtasks} />);

    expect(screen.getByText('Task')).toBeInTheDocument();
    expect(screen.queryByRole('button', { expanded: false })).not.toBeInTheDocument();
  });
});
