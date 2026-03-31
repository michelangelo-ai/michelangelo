import { forwardRef } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';
import { ACCESSIBILITY_TYPE, PLACEMENT, StatefulTooltip } from 'baseui/tooltip';

import { Icon } from '#core/components/icon/icon';

import type { MenuAdapterProps } from 'baseui/list';
import type { ComponentActionConfig, Data, SelectedAction } from '#core/components/actions/types';

type ActionMenuItemProps = {
  /**
   * Item is the action configuration defined for a specific action in
   * the ActionMenu list, passed as `item` per baseui MenuAdapter props.
   */
  item: ComponentActionConfig;
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
} & Omit<MenuAdapterProps, 'children' | 'item'>;

export const ActionMenuItem = forwardRef<HTMLLIElement, ActionMenuItemProps>((props, ref) => {
  const { item: action, record, onSelectAction, ...baseMenuProps } = props;

  const disabledRule = action.disabled?.find((rule) => rule.condition(record));
  const isDisabled = !!disabledRule;

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
      overrides={{ Root: { style: { height: '44px' } } }}
      $disabled={isDisabled}
      onClick={
        isDisabled ? undefined : () => onSelectAction({ component: action.component, record })
      }
    >
      <ListItemLabel>{action.display.label}</ListItemLabel>
    </MenuAdapter>
  );

  if (!isDisabled || !disabledRule.message) return menuItem;

  return (
    <StatefulTooltip
      content={disabledRule.message}
      accessibilityType={ACCESSIBILITY_TYPE.tooltip}
      showArrow
      placement={PLACEMENT.top}
    >
      {/* Wrapper div required for BaseUI tooltip event delegation */}
      <div>{menuItem}</div>
    </StatefulTooltip>
  );
});

ActionMenuItem.displayName = 'ActionMenuItem';
