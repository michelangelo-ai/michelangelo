import { StatefulMenu } from 'baseui/menu';

import { ActionMenuItem } from './action-menu-item';

import type { Theme } from 'baseui';
import type { ActionSchema, Data } from '#core/components/actions/types';

type ActionMenuProps = {
  items: ActionSchema[];
  record: Data;
};

export function ActionMenu(props: ActionMenuProps) {
  return (
    <StatefulMenu
      items={props.items}
      overrides={{
        Option: {
          component: ActionMenuItem,
          props: { record: props.record },
        },
        List: { style: ({ $theme }: { $theme: Theme }) => ({ padding: $theme.sizing.scale600 }) },
      }}
    />
  );
}
