import { useStyletron } from 'baseui';

import { InterpolatableActionsPopover } from '#core/components/actions/interpolatable-actions-popover';
import { Row } from '#core/components/row/row';
import { TaskContentStack } from '#core/components/views/execution/styled-components';
import { TaskStateIcon } from '../task-state-icon';

import type { TaskHeaderProps } from './types';

/**
 * Task header component showing icon, name, and metadata.
 */
export function TaskHeader<TTaskRecord extends object>(props: TaskHeaderProps<TTaskRecord>) {
  const [css, theme] = useStyletron();
  const { task, id, metadata, actions, pageData } = props;
  const { name, state } = task;
  // cast: TTaskRecord extends object lacks an index signature; always a plain record at
  // runtime; see #1443
  const record = task.record as Record<string, unknown>;
  const actionRecord = { ...pageData, ...record };

  return (
    <TaskContentStack id={id}>
      <div
        className={css({
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
        })}
      >
        <div className={css({ display: 'flex', gap: theme.sizing.scale500 })}>
          <div className={css({ marginTop: '2px' })}>
            <TaskStateIcon state={state} />
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
        {!!actions?.length && (
          <InterpolatableActionsPopover actions={actions} record={actionRecord} />
        )}
      </div>
      {metadata && <Row items={metadata} record={record} />}
    </TaskContentStack>
  );
}
