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
  const { task, metadata, actions, pageData, bodySchema, overrides } = props;
  const scrollId = buildTaskScrollId(task);
  // Top-level steps (e.g. Image Build, Execute Workflow) are actor-driven and have no
  // per-task actions available; only nested tasks support them.
  const taskActions = task.depth > 0 ? actions : undefined;

  if (!!task.subTasks?.length || bodySchema?.length) {
    // cast: TTaskRecord extends object lacks an index signature; always a plain record at
    // runtime; see #1443
    const record = task.record as Record<string, unknown>;
    const actionRecord = { ...pageData, ...record };

    return (
      <TaskPanel
        id={scrollId}
        title={<TaskHeader task={task} metadata={metadata} />}
        defaultExpanded={task.focused}
        state={task.state}
        actions={taskActions}
        actionRecord={actionRecord}
      >
        <TaskBody
          task={task}
          bodySchema={bodySchema}
          metadata={metadata}
          actions={actions}
          pageData={pageData}
          overrides={overrides}
        />
      </TaskPanel>
    );
  }

  return (
    <TaskHeader
      id={scrollId}
      task={task}
      metadata={metadata}
      actions={taskActions}
      pageData={pageData}
    />
  );
}
