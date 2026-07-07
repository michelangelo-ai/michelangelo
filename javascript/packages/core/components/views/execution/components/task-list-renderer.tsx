import React from 'react';

import { TaskIndicator } from './styled-components';
import { TaskStepCard } from './task-step-card/task-step-card';

import type { TaskListRendererProps } from './types';

/**
 * Default task list renderer for execution views.
 * Renders tasks as horizontal step cards with indicators between them.
 */
export function TaskListRenderer<TTaskRecord extends object = object>({
  taskList,
  onTaskClick,
}: TaskListRendererProps<TTaskRecord>) {
  return (
    <>
      {taskList.map((task, taskIndex) => (
        <React.Fragment key={taskIndex}>
          {taskIndex > 0 && <TaskIndicator $color="contentInverseSecondary" $direction="right" />}
          <TaskStepCard
            task={task}
            {...(onTaskClick ? { onClick: () => onTaskClick(task) } : {})}
          />
        </React.Fragment>
      ))}
    </>
  );
}
