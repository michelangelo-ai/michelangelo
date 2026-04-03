import { useMemo, useState } from 'react';
import { StatefulMenu } from 'baseui/menu';

import { ActionMenuItem } from './action-menu-item';

import type { Theme } from 'baseui';
import type {
  ActionConfig,
  ComponentActionConfig,
  Data,
  SelectedAction,
} from '#core/components/actions/types';

type ActionMenuProps = {
  actions: ActionConfig[];
  record: Data;
  onSelectAction: (action: SelectedAction) => void;
  onClose?: () => void;
};

export function ActionMenu(props: ActionMenuProps) {
  const [hoveredItem, setHoveredItem] = useState<object | null>(null);
  const [keyboardActive, setKeyboardActive] = useState(false);

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
    // Wrap in a div so our keydown handler fires via bubbling AFTER StatefulMenu's
    // arrow-key handler on the <ul>. Putting onKeyDown directly on the List override
    // props replaces StatefulMenu's handler (later spread wins), breaking navigation.
    <div
      onKeyDown={() => {
        setHoveredItem(null);
        setKeyboardActive(true);
      }}
    >
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
              keyboardActive,
              setKeyboardActive,
            },
          },
          List: {
            style: ({ $theme }: { $theme: Theme }) => ({ padding: $theme.sizing.scale600 }),
          },
        }}
      />
    </div>
  );
}
