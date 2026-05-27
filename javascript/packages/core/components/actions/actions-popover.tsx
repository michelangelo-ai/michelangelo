import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { PLACEMENT, StatefulPopover } from 'baseui/popover';

import { Icon } from '#core/components/icon/icon';
import { ActionDispatcher } from './action-dispatcher';
import { ActionMenu } from './action-menu/action-menu';
import { useResolvedActionItems } from './use-resolved-action-items';

import type { ButtonProps } from 'baseui/button';
import type { BasePopoverProps } from 'baseui/popover';
import type { ActionConfig, Data } from './types';

type ActionsPopoverProps<T extends Data> = {
  actions: ActionConfig<T>[];
  buttonProps?: ButtonProps;
  record: T;
  popoverProps?: BasePopoverProps;
};

export function ActionsPopover<T extends Data>({
  actions,
  buttonProps,
  record,
  popoverProps,
}: ActionsPopoverProps<T>) {
  const scrollDisabledRef = useRef(false);
  const [activeAction, setActiveAction] = useState<ActionConfig<T> | null>(null);
  const navigate = useNavigate();
  const [, theme] = useStyletron();

  const items = useResolvedActionItems(actions, (action) => {
    if (action.modal) {
      setActiveAction(action);
    } else if (action.action?.type === 'route') {
      navigate(action.action.route);
    }
  });

  const disableScroll = () => {
    document.body.style.overflow = 'hidden';
    scrollDisabledRef.current = true;
  };

  const enableScroll = () => {
    document.body.style.overflow = '';
    scrollDisabledRef.current = false;
  };

  useEffect(() => {
    return () => {
      if (scrollDisabledRef.current) {
        document.body.style.overflow = '';
      }
    };
  }, []);

  return (
    <>
      <StatefulPopover
        focusLock
        placement={PLACEMENT.bottomLeft}
        popperOptions={{
          modifiers: {
            preventOverflow: { enabled: true, boundariesElement: 'window', padding: 8 },
          },
        }}
        {...popoverProps}
        content={({ close }) => (
          <ActionMenu
            actions={items}
            onSelectAction={(action) => {
              action.onClick();
              close();
            }}
            onClose={close}
          />
        )}
        onClose={enableScroll}
        onOpen={disableScroll}
      >
        <Button
          kind={KIND.tertiary}
          shape={SHAPE.pill}
          overrides={{
            BaseButton: {
              style: { paddingLeft: theme.sizing.scale100, paddingRight: theme.sizing.scale100 },
            },
          }}
          {...buttonProps}
          size={SIZE.compact}
          title="Actions"
          data-tracking-name="actions-popover-button"
        >
          <Icon name="overflowMenu" />
        </Button>
      </StatefulPopover>
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
