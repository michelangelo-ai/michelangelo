import React from 'react';

import { TaskFlowContainer, TaskIndicator } from './styled-components';
import { TaskStepCard } from './task-step-card/task-step-card';

import type { Task } from '#core/components/views/execution/types';

export function TaskFlow<TTaskRecord extends object = object>(props: {
  taskList: Task<TTaskRecord>[];
  onTaskClick?: (task: Task<TTaskRecord>) => void;
}) {
  const { taskList, onTaskClick } = props;

  return (
    <TaskFlowContainer>
      {taskList.map((task, index) => (
        <React.Fragment key={index}>
          {index > 0 && <TaskIndicator $color="contentInverseSecondary" $direction="right" />}
          <TaskStepCard key={index} task={task} onClick={() => onTaskClick?.(task)} />
        </React.Fragment>
      ))}
    </TaskFlowContainer>
  );
}
