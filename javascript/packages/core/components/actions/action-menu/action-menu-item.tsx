import { forwardRef } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';

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

  return (
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
      onClick={() => onSelectAction({ component: action.component, record })}
    >
      <ListItemLabel>{action.display.label}</ListItemLabel>
    </MenuAdapter>
  );
});

ActionMenuItem.displayName = 'ActionMenuItem';
