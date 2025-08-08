import { useStyletron } from 'baseui';

import { TaskIndicator } from '#core/components/views/execution/components/styled-components';
import { TaskStateIcon } from '#core/components/views/execution/components/task-state-icon';
import { TaskStepCardContainer, TaskStepName } from './styled-components';

import type { TaskStepCardProps } from './types';

export function TaskStepCard<TTaskRecord extends object = object>(
  props: TaskStepCardProps<TTaskRecord>
) {
  const { task, onClick } = props;
  const { focused, name, state, subTasks } = task;
  const hasSubTasks = !!subTasks?.length;
  const [css] = useStyletron();

  return (
    <TaskStepCardContainer $state={state} role="button" tabIndex={0} onClick={onClick}>
      <TaskStateIcon state={state} />
      <TaskStepName>{name}</TaskStepName>
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
    </TaskStepCardContainer>
  );
}
