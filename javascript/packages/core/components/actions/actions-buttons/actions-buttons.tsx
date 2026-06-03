import { useState } from 'react';
import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { PLACEMENT } from 'baseui/popover';

import { ActionDispatcher } from '#core/components/actions/action-dispatcher';
import { ActionsPopover } from '#core/components/actions/actions-popover';
import { Icon } from '#core/components/icon/icon';
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
          <Button
            kind={KIND.primary}
            size={SIZE.compact}
            isLoading={loading}
            overrides={{ Root: { style: { width: '200px' } } }}
            startEnhancer={
              primary.display.icon
                ? () => (
                    <Icon
                      name={primary.display.icon}
                      size={theme.sizing.scale550}
                      color="inherit"
                    />
                  )
                : undefined
            }
            onClick={() => activateAction(primary)}
          >
            {primary.display.label}
          </Button>
        )}
        {secondary.map((action) => (
          <Button
            key={action.display.label}
            kind={KIND.secondary}
            shape={SHAPE.pill}
            size={SIZE.compact}
            isLoading={loading}
            startEnhancer={
              action.display.icon
                ? () => (
                    <Icon name={action.display.icon} size={theme.sizing.scale550} color="inherit" />
                  )
                : undefined
            }
            onClick={() => activateAction(action)}
          >
            {action.display.label}
          </Button>
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
