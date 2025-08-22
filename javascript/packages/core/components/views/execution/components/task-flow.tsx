import React from 'react';

import { TaskSeparator } from '../styled-components';
import { TaskFlowContainer, TaskIndicator } from './styled-components';
import { TaskStepCard } from './task-step-card/task-step-card';

import type { TaskFlowProps } from './task-flow-types';

export function TaskFlow<TTaskRecord extends object = object>({
  matrix,
  onTaskClick,
}: TaskFlowProps<TTaskRecord>) {
  return (
    <>
      {matrix.map((item, index) => (
        <React.Fragment key={index}>
          {index > 0 && <TaskSeparator />}
          <TaskFlowContainer>
            {item.taskList.map((task, taskIndex) => (
              <React.Fragment key={taskIndex}>
                {taskIndex > 0 && (
                  <TaskIndicator $color="contentInverseSecondary" $direction="right" />
                )}
                <TaskStepCard
                  task={task}
                  {...(onTaskClick ? { onClick: () => onTaskClick(task) } : {})}
                />
              </React.Fragment>
            ))}
          </TaskFlowContainer>
        </React.Fragment>
      ))}
    </>
  );
}
