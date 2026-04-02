import { useMemo, useState } from 'react';
import { StatefulMenu } from 'baseui/menu';

import { ActionMenuItem } from './action-menu-item';

import type { Theme } from 'baseui';
import type { ComponentActionConfig } from '#core/components/actions/types';
import type { ActionConfig, Data, SelectedAction } from '#core/components/actions/types';

type ActionMenuProps = {
  actions: ActionConfig[];
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
  onClose?: () => void;
};

export function ActionMenu(props: ActionMenuProps) {
  const [hoveredItem, setHoveredItem] = useState<object | null>(null);

  // ActionConfig.disabled is a rule array; StatefulMenu expects a boolean item.disabled.
  // Pre-compute here and carry the message forward for ActionMenuItem's tooltip.
  const items = useMemo(
    () =>
      props.actions.map((action) => {
        const disabledRule = action.disabled?.find((rule) => rule.condition(props.record));
        return { ...action, disabled: !!disabledRule, disabledMessage: disabledRule?.message };
      }),
    [props.actions, props.record]
  );

  return (
    <StatefulMenu
      items={items}
      onItemSelect={({ item: action }: { item: ComponentActionConfig }) => {
        props.onSelectAction({ component: action.component, record: props.record });
      }}
      overrides={{
        Option: {
          component: ActionMenuItem,
          props: {
            record: props.record,
            onSelectAction: props.onSelectAction,
            onClose: props.onClose,
            hoveredItem,
            setHoveredItem,
          },
        },
        List: {
          // Clear hover on any keypress so keyboard navigation doesn't leave a stale hover tooltip open.
          props: { onKeyDown: () => setHoveredItem(null) },
          style: ({ $theme }: { $theme: Theme }) => ({ padding: $theme.sizing.scale600 }),
        },
      }}
    />
  );
}
