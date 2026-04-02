import { forwardRef } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';
import { ACCESSIBILITY_TYPE, PLACEMENT, Tooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';

import type { MenuAdapterProps } from 'baseui/list';
import type { ComponentActionConfig, Data, SelectedAction } from '#core/components/actions/types';

type ActionMenuItemProps = {
  // action-menu.tsx pre-computes disabled state before passing items to BaseUI —
  // ActionConfig.disabled is a rule array; StatefulMenu expects a boolean.
  item: Omit<ComponentActionConfig, 'disabled'> & {
    disabled: boolean;
    disabledMessage: string | undefined;
  };
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
  onClose?: () => void;
  hoveredItem: object | null;
  setHoveredItem: (item: object | null) => void;
} & Omit<MenuAdapterProps, 'children' | 'item'>;

export const ActionMenuItem = forwardRef<HTMLLIElement, ActionMenuItemProps>((props, ref) => {
  const { item: action, record, onSelectAction, onClose, hoveredItem, setHoveredItem, ...baseMenuProps } = props;
  const isHovered = hoveredItem === action;

  const menuItem = (
    <MenuAdapter
      {...baseMenuProps}
      ref={ref}
      role="option"
      artwork={
        action.display.icon
          ? ({ size }: { size: number }) => <Icon name={action.display.icon} size={`${size}px`} />
          : undefined
      }
      artworkSize={ARTWORK_SIZES.MEDIUM}
      overrides={{ Root: { style: { height: '44px', opacity: action.disabled ? '0.4' : '1' } } }}
      $disabled={action.disabled}
      onClick={
        action.disabled ? undefined : () => onSelectAction({ component: action.component, record })
      }
    >
      <ListItemLabel>{action.display.label}</ListItemLabel>
    </MenuAdapter>
  );

  if (!action.disabled || !action.disabledMessage) return menuItem;

  return (
    <Tooltip
      content={action.disabledMessage}
      autoFocus={false}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      showArrow
      placement={PLACEMENT.left}
      isOpen={!!baseMenuProps.$isHighlighted || isHovered}
      onMouseEnter={() => setHoveredItem(action)}
      onMouseLeave={() => setHoveredItem(null)}
      popperOptions={{
        modifiers: {
          flip: { enabled: false }, // respect the placement prop; flip would override it
          preventOverflow: { enabled: true, boundariesElement: 'window', padding: 8 },
        },
      }}
      overrides={{ Body: { style: { pointerEvents: 'none' } } }}
      onEsc={onClose}
    >
      {/* Wrapper div required for BaseUI tooltip event delegation */}
      <div>{menuItem}</div>
    </Tooltip>
  );
});

ActionMenuItem.displayName = 'ActionMenuItem';
