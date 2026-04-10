import { render, screen } from '@testing-library/react';
import { ArrowUp, Check, Delete } from 'baseui/icon';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { TASK_STATE } from '../../../constants';
import { createTask } from '../__fixtures__/task-details-fixtures';
import { TaskHeader } from '../task-header';

describe('TaskHeader', () => {
  it('should display task name and state icon', () => {
    const task = createTask({ name: 'Build Pipeline', state: TASK_STATE.RUNNING });

    render(
      <TaskHeader task={task} />,
      buildWrapper([
        getIconProviderWrapper({
          icons: {
            arrowCircular: ArrowUp,
          },
        }),
      ])
    );

    expect(screen.getByText('Build Pipeline')).toBeInTheDocument();
    expect(screen.getByText('Arrow Up')).toBeInTheDocument();
  });

  it('should render task name and metadata together', () => {
    const task = createTask({
      name: 'Task with Metadata',
      state: TASK_STATE.RUNNING,
      record: {
        status: 'RUNNING',
        duration: '120s',
        startTime: '2025-01-01T10:00:00Z',
      },
    });

    render(<TaskHeader task={task} metadata={[
        { id: 'status', label: 'Status' },
        { id: 'duration', label: 'Duration' },
        { id: 'startTime', label: 'Started' },
      ]} />);

    expect(screen.getByText('Task with Metadata')).toBeInTheDocument();
    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Duration')).toBeInTheDocument();
    expect(screen.getByText('Started')).toBeInTheDocument();
  });

  it('should handle missing metadata gracefully', () => {
    const task = createTask({ name: 'Task without Metadata' });

    render(<TaskHeader task={task} />);

    expect(screen.getByText('Task without Metadata')).toBeInTheDocument();
    expect(screen.queryByText('Status')).not.toBeInTheDocument();
    expect(screen.queryByText('Duration')).not.toBeInTheDocument();
  });

  it('should handle empty metadata array', () => {
    const task = createTask({ name: 'Task with Empty Metadata' });

    render(<TaskHeader task={task} metadata={[]} />);

    expect(screen.getByText('Task with Empty Metadata')).toBeInTheDocument();
    expect(screen.queryByText('Status')).not.toBeInTheDocument();
    expect(screen.queryByText('Duration')).not.toBeInTheDocument();
  });

  it('should display different task states with appropriate icons', () => {
    const errorTask = createTask({ name: 'Failed Task', state: TASK_STATE.ERROR });
    const successTask = createTask({ name: 'Success Task', state: TASK_STATE.SUCCESS });

    const { rerender } = render(
      <TaskHeader task={errorTask} />,
      buildWrapper([
        getIconProviderWrapper({
          icons: {
            circleX: Delete, // For ERROR state
            circleCheck: Check, // For SUCCESS state
          },
        }),
      ])
    );

    expect(screen.getByText('Failed Task')).toBeInTheDocument();
    expect(screen.getByText('Delete')).toBeInTheDocument();

    rerender(<TaskHeader task={successTask} />);
    expect(screen.getByText('Success Task')).toBeInTheDocument();
    expect(screen.getByText('Check')).toBeInTheDocument();
  });

  it('should handle missing record fields gracefully', () => {
    const task = createTask({
      name: 'Incomplete Task',
      state: TASK_STATE.PENDING,
      record: { displayName: 'Incomplete Task' }, // Missing status, duration, startTime
    });

    render(<TaskHeader task={task} metadata={[
        { id: 'status', label: 'Status' },
        { id: 'duration', label: 'Duration' },
        { id: 'startTime', label: 'Started' },
      ]} />);

    expect(screen.getByText('Incomplete Task')).toBeInTheDocument();

    expect(screen.getByText('Status')).toBeInTheDocument();
    expect(screen.getByText('Duration')).toBeInTheDocument();
    expect(screen.getByText('Started')).toBeInTheDocument();
  });
});
