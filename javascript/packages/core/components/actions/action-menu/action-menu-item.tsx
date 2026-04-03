import { forwardRef } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';
import { ACCESSIBILITY_TYPE, PLACEMENT, Tooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';

import type { MenuAdapterProps } from 'baseui/list';
import type { ComponentActionConfig, Data, SelectedAction } from '#core/components/actions/types';

type ActionMenuItemProps = {
  item: Omit<ComponentActionConfig, 'disabled'> & {
    disabled: boolean;
    disabledMessage: string | undefined;
  };
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
  onClose?: () => void;
  hoveredItem: object | null;
  setHoveredItem: (item: object | null) => void;
  keyboardActive: boolean;
  setKeyboardActive: (active: boolean) => void;
} & Omit<MenuAdapterProps, 'children' | 'item'>;

export const ActionMenuItem = forwardRef<HTMLLIElement, ActionMenuItemProps>((props, ref) => {
  const {
    item: action,
    record,
    onSelectAction,
    onClose,
    hoveredItem,
    setHoveredItem,
    keyboardActive,
    setKeyboardActive,
    ...baseMenuProps
  } = props;
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
      isOpen={(!!baseMenuProps.$isHighlighted && keyboardActive) || isHovered}
      popperOptions={{
        modifiers: {
          flip: { enabled: false }, // respect the placement prop; flip would override it
          preventOverflow: { enabled: true, boundariesElement: 'window', padding: 8 },
        },
      }}
      onEsc={onClose}
      // Pass hover handlers as Tooltip props — BaseUI's Popover calls these from
      // onAnchorMouseEnter/onAnchorMouseLeave. Placing them on the wrapper div
      // does NOT work because Popover's cloneElement replaces div-level handlers.
      onMouseEnterDelay={0}
      onMouseLeaveDelay={0}
      onMouseEnter={() => {
        setHoveredItem(action);
        setKeyboardActive(false);
      }}
      onMouseLeave={() => setHoveredItem(null)}
    >
      {menuItem}
    </Tooltip>
  );
});

ActionMenuItem.displayName = 'ActionMenuItem';
