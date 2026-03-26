import { forwardRef, useContext } from 'react';
import { ARTWORK_SIZES, ListItemLabel, MenuAdapter } from 'baseui/list';

import { ActionMenuContext } from '#core/components/actions/context';
import { Icon } from '#core/components/icon/icon';

import type { MenuAdapterProps } from 'baseui/list';
import type { ComponentActionSchema, Data } from '#core/components/actions/types';

type ActionMenuItemProps = { item: ComponentActionSchema; record: Data } & Omit<
  MenuAdapterProps,
  'children' | 'item'
>;

export const ActionMenuItem = forwardRef<HTMLLIElement, ActionMenuItemProps>((props, ref) => {
  const context = useContext(ActionMenuContext);
  const { item, record, ...baseMenuProps } = props;

  return (
    <MenuAdapter
      // MenuAdapter is a thin wrapper around BaseWeb's list components that adds
      // support for artwork & handles interaction states & accessibility. The props
      // forwarding is required boilerplate to get the aforementioned benefits.
      {...baseMenuProps}
      ref={ref}
      role="option"
      artwork={
        item.display.icon
          ? ({ size }: { size: number }) => <Icon name={item.display.icon} size={`${size}px`} />
          : undefined
      }
      artworkSize={ARTWORK_SIZES.MEDIUM}
      overrides={{ Root: { style: { height: '44px' } } }}
      onClick={() => context.openAction({ component: item.component, record })}
    >
      <ListItemLabel>{item.display.label}</ListItemLabel>
    </MenuAdapter>
  );
});

ActionMenuItem.displayName = 'ActionMenuItem';
