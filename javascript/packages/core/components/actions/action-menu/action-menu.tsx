import { useMemo } from 'react';
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
  // BaseUI checks item.disabled (boolean) to gate onItemSelect (Enter key) and onMouseEnter
  // (hover highlighting). Our ActionConfig.disabled is an array of rules, so we pre-compute
  // the boolean here and carry the message forward for ActionMenuItem's tooltip.
  const items = useMemo(
    () =>
      props.actions.map((action) => {
        const a = action as ComponentActionConfig;
        const disabledRule = a.disabled?.find((rule) => rule.condition(props.record));
        return { ...a, disabled: !!disabledRule, disabledMessage: disabledRule?.message };
      }),
    [props.actions, props.record]
  );

  return (
    <StatefulMenu
      items={items}
      onItemSelect={({ item }) => {
        const action = item as ComponentActionConfig;
        props.onSelectAction({ component: action.component, record: props.record });
      }}
      overrides={{
        Option: {
          component: ActionMenuItem,
          props: {
            record: props.record,
            onSelectAction: props.onSelectAction,
            onClose: props.onClose,
          },
        },
        List: { style: ({ $theme }: { $theme: Theme }) => ({ padding: $theme.sizing.scale600 }) },
      }}
    />
  );
}
