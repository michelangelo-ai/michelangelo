import { TaskPanel } from '#core/components/views/execution/styled-components';
import { TaskBody } from './task-body';
import { TaskHeader } from './task-header';

import type { TaskDetailsProps } from './types';

/**
 * Accordion-style task display component following the source task-details pattern.
 * Shows TaskHeader always, and TaskBody (with subtasks) when accordion is expanded.
 * For leaf tasks (no subtasks), shows simple TaskHeader only.
 *
 * @param task - The task data to display
 * @param onClick - Optional click handler for task interaction
 * @param metadata - Optional metadata field configurations for rich display
 * @param bodySchema - Optional body content schema for leaf tasks
 */
export function TaskDetails<TTaskRecord extends object = object>(
  props: TaskDetailsProps<TTaskRecord>
) {
  const { task, onClick, metadata, bodySchema } = props;

  if (!!task.subTasks?.length || bodySchema?.length) {
    return (
      <TaskPanel
        title={<TaskHeader task={task} onClick={onClick} metadata={metadata} />}
        initialState={{ expanded: false }}
      >
        <TaskBody task={task} bodySchema={bodySchema} />
      </TaskPanel>
    );
  }

  return <TaskHeader task={task} onClick={onClick} metadata={metadata} />;
}
