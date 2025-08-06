import { StatefulPanel } from 'baseui/accordion';

import { TaskBody } from './task-body';
import { TaskHeader } from './task-header';

import type { Task } from '../../types';
import type { TaskDetailsProps } from './types';

/**
 * Accordion-style task display component following the source task-details pattern.
 * Shows TaskHeader always, and TaskBody (with subtasks) when accordion is expanded.
 * For leaf tasks (no subtasks), shows simple TaskHeader only.
 *
 * @param task - The task data to display
 * @param onClick - Optional click handler for task interaction
 * @param metadata - Optional metadata field configurations for rich display
 */
export function TaskDetails<TTaskRecord extends object = object>(
  props: TaskDetailsProps<TTaskRecord>
) {
  const { task, onClick, metadata } = props;

  if (shouldRenderBody(task)) {
    return (
      <StatefulPanel
        title={<TaskHeader task={task} onClick={onClick} metadata={metadata} />}
        initialState={{ expanded: false }}
      >
        <TaskBody task={task} />
      </StatefulPanel>
    );
  }

  return <TaskHeader task={task} onClick={onClick} metadata={metadata} />;
}

/**
 * Determines if task should render accordion body (when it has subtasks).
 */
function shouldRenderBody<TTaskRecord extends object>(task: Task<TTaskRecord>): boolean {
  return !!task.subTasks?.length;
}
