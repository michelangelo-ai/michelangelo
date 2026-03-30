import { StatefulMenu } from 'baseui/menu';

import { ActionMenuItem } from './action-menu-item';

import type { Theme } from 'baseui';
import type { ActionConfig, Data, SelectedAction } from '#core/components/actions/types';

type ActionMenuProps = {
  actions: ActionConfig[];
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
};

export function ActionMenu(props: ActionMenuProps) {
  return (
    <StatefulMenu
      items={props.actions}
      overrides={{
        Option: {
          component: ActionMenuItem,
          props: { record: props.record, onSelectAction: props.onSelectAction },
        },
        List: { style: ({ $theme }: { $theme: Theme }) => ({ padding: $theme.sizing.scale600 }) },
      }}
    />
  );
}
