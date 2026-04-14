import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { PLACEMENT } from 'baseui/popover';

import { ActionsPopover } from '#core/components/actions/actions-popover';
import { Icon } from '#core/components/icon/icon';
import { partitionActions } from './utils';

import type { ActionConfig, Data, SelectedAction } from '#core/components/actions/types';

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
  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);

  if (actions.length === 0) return null;

  const { primary, secondary, tertiary } = partitionActions(actions);
  const ActiveComponent = selectedAction?.component;

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
            onClick={() => setSelectedAction({ component: primary.component, record })}
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
            onClick={() => setSelectedAction({ component: action.component, record })}
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
      {selectedAction && ActiveComponent && (
        <ActiveComponent
          record={selectedAction.record}
          isOpen
          onClose={() => setSelectedAction(null)}
        />
      )}
    </>
  );
}
