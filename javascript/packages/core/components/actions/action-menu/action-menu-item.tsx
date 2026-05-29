import { forwardRef } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';
import { ACCESSIBILITY_TYPE, PLACEMENT, Tooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';

import type { MenuAdapterProps } from 'baseui/list';
import type { ResolvedActionItem } from '#core/components/actions/types';

type ActionMenuItemProps = {
  /** Named `item` to match BaseUI's MenuAdapter injection. */
  item: ResolvedActionItem;
  onSelectAction: (action: ResolvedActionItem) => void;
  onClose?: () => void;
  /**
   * The action currently under the mouse cursor, or null.
   * Compared by object identity against `action` to derive `isHovered`.
   */
  hoveredItem: object | null;
  setHoveredItem: (item: object | null) => void;
  /**
   * True after any keydown inside the menu. False on mouse enter.
   * Gates the keyboard tooltip path so auto-highlight on focus
   * doesn't flash a tooltip.
   */
  keyboardActive: boolean;
  setKeyboardActive: (active: boolean) => void;
} & Omit<MenuAdapterProps, 'children' | 'item'>;

export const ActionMenuItem = forwardRef<HTMLLIElement, ActionMenuItemProps>((props, ref) => {
  const {
    item: action,
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
      // MenuAdapter is a thin wrapper around BaseWeb's list components that adds
      // support for artwork & handles interaction states & accessibility. The props
      // forwarding is required boilerplate to get the aforementioned benefits.
      {...baseMenuProps}
      ref={ref}
      role="option"
      artwork={
        action.display.icon
          ? ({ size }: { size: number }) => <Icon name={action.display.icon} size={`${size}px`} />
          : undefined
      }
      artworkSize={ARTWORK_SIZES.MEDIUM}
      // Opacity rather than $theme.colors.menuFontDisabled because ListItemLabel's
      // <p> sets its own color (contentPrimary), blocking CSS inheritance from the <li>.
      // Opacity dims the entire item (icon + text) uniformly.
      overrides={{ Root: { style: { height: '44px', opacity: action.disabled ? 0.4 : 1 } } }}
      $disabled={action.disabled}
      onClick={action.disabled ? undefined : () => onSelectAction(action)}
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
      // Keyboard path: only when arrow keys were used (not auto-highlight on focus).
      // Mouse path: only when this specific item is hovered (object identity).
      isOpen={(!!baseMenuProps.$isHighlighted && keyboardActive) || isHovered}
      popperOptions={{
        modifiers: {
          flip: { enabled: false }, // respect the placement prop; flip would override it
          preventOverflow: { enabled: true, boundariesElement: 'window', padding: 8 },
        },
      }}
      onEsc={onClose}
      onMouseEnterDelay={0}
      onMouseLeaveDelay={0}
      // Entering mouse mode: track this item as hovered and disable the keyboard
      // path so the previously arrow-key-highlighted item's tooltip hides.
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
