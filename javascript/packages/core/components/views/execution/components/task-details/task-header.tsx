import { useStyletron } from 'baseui';

import { Row } from '#core/components/row/row';
import { TaskStateIcon } from '../task-state-icon';

import type { TaskHeaderProps } from './types';

/**
 * Task header component showing icon, name, and metadata.
 */
export function TaskHeader<TTaskRecord extends object>(props: TaskHeaderProps<TTaskRecord>) {
  const [css, theme] = useStyletron();
  const { task, metadata } = props;
  const { name, state } = task;

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale800 })}>
      <div className={css({ display: 'flex', gap: theme.sizing.scale500 })}>
        <div className={css({ marginTop: '2px' })}>
          <TaskStateIcon state={state} size={20} />
        </div>
        <div
          className={css({
            ...theme.typography.LabelLarge,
            marginBottom: theme.sizing.scale100,
          })}
        >
          {name}
        </div>
      </div>
      {metadata && <Row items={metadata} record={task.record as Record<string, unknown>} />}
    </div>
  );
}
