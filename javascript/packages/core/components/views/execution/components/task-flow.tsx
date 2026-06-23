import React from 'react';
import { getOverrides } from 'baseui';

import { TaskSeparator } from '../styled-components';
import { TaskFlowContainer } from './styled-components';
import { TaskListRenderer } from './task-list-renderer';

import type { TaskFlowProps } from './types';

export function TaskFlow<TTaskRecord extends object = object>({
  matrix,
  onTaskClick,
  overrides,
}: TaskFlowProps<TTaskRecord>) {
  const [TaskListRendererComponent, taskListRendererProps] = getOverrides(
    overrides?.TaskListRenderer,
    TaskListRenderer
  );
  const [SubTaskListRendererComponent, subTaskListRendererProps] = getOverrides(
    overrides?.SubTaskListRenderer ?? overrides?.TaskListRenderer,
    TaskListRenderer
  );

  return (
    <>
      {matrix.map((item, index) => {
        const Renderer = item.parent ? SubTaskListRendererComponent : TaskListRendererComponent;
        const rendererProps = item.parent ? subTaskListRendererProps : taskListRendererProps;
        return (
          <React.Fragment key={index}>
            {index > 0 && <TaskSeparator />}
            <TaskFlowContainer>
              <Renderer
                taskList={item.taskList}
                onTaskClick={onTaskClick}
                parent={item.parent}
                {...rendererProps}
              />
            </TaskFlowContainer>
          </React.Fragment>
        );
      })}
    </>
  );
}
