import { TaskPanel } from '#core/components/views/execution/styled-components';
import { buildTaskScrollId } from '#core/components/views/execution/utils/scroll-to-task';
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
  const { task, metadata, bodySchema } = props;
  const scrollId = buildTaskScrollId(task);

  if (!!task.subTasks?.length || bodySchema?.length) {
    return (
      <TaskPanel
        id={scrollId}
        title={<TaskHeader task={task} metadata={metadata} />}
        initialState={{ expanded: task.focused }}
      >
        <TaskBody task={task} bodySchema={bodySchema} />
      </TaskPanel>
    );
  }

  return <TaskHeader id={scrollId} task={task} metadata={metadata} />;
}
