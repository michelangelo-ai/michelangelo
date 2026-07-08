import { useState } from 'react';
import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';
import { KIND, SHAPE } from 'baseui/button';
import { PLACEMENT } from 'baseui/popover';

import { ActionDispatcher } from '#core/components/actions/action-dispatcher';
import { ActionsPopover } from '#core/components/actions/actions-popover';
import { ActionButton } from './action-button';
import { partitionActions } from './utils';

import type { ActionConfig, Data } from '#core/components/actions/types';

type ActionsButtonsProps<T extends Data = Data> = {
  actions: ActionConfig<T>[];
  record: T;
  loading?: boolean;
};

/**
 * Renders action buttons partitioned by hierarchy level and manages the selected action.
 *
 * Primary actions render as a fixed-width filled button, secondary as pill-shaped
 * buttons, and tertiary actions collapse into an overflow popover.
 */
export function ActionsButtons<T extends Data>({
  actions,
  record,
  loading,
}: ActionsButtonsProps<T>) {
  const [css, theme] = useStyletron();
  const [activeAction, setActiveAction] = useState<ActionConfig<T> | null>(null);
  const navigate = useNavigate();

  if (actions.length === 0) return null;

  const { primary, secondary, tertiary } = partitionActions(actions);

  const activateAction = (action: ActionConfig<T>) => {
    if (action.modal) {
      setActiveAction(action);
    } else if (action.operation?.type === 'route') {
      navigate(action.operation.route);
    }
  };

  return (
    <>
      <div className={css({ display: 'flex', gap: theme.sizing.scale300 })}>
        {primary && (
          <ActionButton
            action={primary}
            onClick={() => activateAction(primary)}
            loading={loading}
            kind={KIND.primary}
            overrides={{ Root: { style: { width: '200px' } } }}
          />
        )}
        {secondary.map((action) => (
          <ActionButton
            key={action.display.label}
            action={action}
            onClick={() => activateAction(action)}
            loading={loading}
            kind={KIND.secondary}
            shape={SHAPE.pill}
          />
        ))}
        {tertiary.length > 0 && (
          <ActionsPopover
            actions={tertiary}
            record={record}
            popoverProps={{ placement: PLACEMENT.bottomRight }}
          />
        )}
      </div>
      {activeAction && (
        <ActionDispatcher
          action={activeAction}
          record={record}
          onClose={() => setActiveAction(null)}
        />
      )}
    </>
  );
}
