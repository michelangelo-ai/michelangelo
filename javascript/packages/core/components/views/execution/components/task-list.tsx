import React from 'react';

import { TaskIndicator, TaskListContainer } from './styled-components';
import { TaskNode } from './task-node/task-node';

import type { Task } from '#core/components/views/execution/types';

export function TaskList<TTaskRecord extends object = object>(props: {
  taskList: Task<TTaskRecord>[];
  onTaskClick?: (task: Task<TTaskRecord>) => void;
}) {
  const { taskList, onTaskClick } = props;

  return (
    <TaskListContainer>
      {taskList.map((task, index) => (
        <React.Fragment key={index}>
          {index > 0 && <TaskIndicator $color="contentInverseSecondary" $direction="right" />}
          <TaskNode key={index} task={task} onClick={() => onTaskClick?.(task)} />
        </React.Fragment>
      ))}
    </TaskListContainer>
  );
}
