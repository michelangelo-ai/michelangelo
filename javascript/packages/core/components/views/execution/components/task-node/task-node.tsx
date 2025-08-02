import { useStyletron } from 'baseui';

import { TaskIndicator } from '#core/components/views/execution/components/styled-components';
import { TaskStateIcon } from '#core/components/views/execution/components/task-state-icon';
import { TaskCard, TaskName } from './styled-components';

import type { TaskNodeProps } from './types';

export function TaskNode<TTaskRecord extends object = object>(props: TaskNodeProps<TTaskRecord>) {
  const { task, onClick } = props;
  const { focused, name, state, subTasks } = task;
  const hasSubTasks = !!subTasks?.length;
  const [css] = useStyletron();

  return (
    <TaskCard $state={state} role="button" tabIndex={0} onClick={onClick}>
      <TaskStateIcon state={state} size={20} />
      <TaskName>{name}</TaskName>
      {hasSubTasks && focused ? (
        <div
          className={css({
            position: 'absolute',
            bottom: '-25px',
            left: '50%',
            transform: 'translate(-50%, 0)',
          })}
        >
          <TaskIndicator $color="contentInverseTertiary" $direction="up" />
        </div>
      ) : null}
    </TaskCard>
  );
}
