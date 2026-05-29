import { useState } from 'react';
import { StatefulMenu } from 'baseui/menu';

import { ActionMenuItem } from './action-menu-item';

import type { Theme } from 'baseui';
import type { ResolvedActionItem } from '#core/components/actions/types';

type ActionMenuProps = {
  actions: ResolvedActionItem[];
  onSelectAction: (action: ResolvedActionItem) => void;
  onClose?: () => void;
};

/**
 * Renders a BaseUI StatefulMenu with tooltip support for disabled items.
 *
 * Disabled items show a tooltip explaining why they're disabled. The tooltip
 * must appear on both mouse hover and keyboard navigation, but NOT on the
 * auto-highlight that StatefulMenu applies to the first item when the menu
 * receives focus. Two pieces of state coordinate this:
 *
 * - `hoveredItem` — tracks which item the mouse is over (object identity).
 *   Ensures only one tooltip is visible at a time without manual cleanup.
 * - `keyboardActive` — distinguishes intentional arrow-key navigation from
 *   the auto-highlight on focus. Set to `true` on any keydown inside the
 *   menu, reset to `false` on mouse enter.
 */
export function ActionMenu({ actions, onSelectAction, onClose }: ActionMenuProps) {
  const [hoveredItem, setHoveredItem] = useState<object | null>(null);
  const [keyboardActive, setKeyboardActive] = useState(false);

  return (
    // Wrapper div: onKeyDown fires via bubbling AFTER StatefulMenu's arrow-key
    // handler on the <ul>. Using List override props would replace that handler
    // (BaseUI's spread puts override props last, so ours wins and theirs is lost).
    <div
      onKeyDown={() => {
        setHoveredItem(null);
        setKeyboardActive(true);
      }}
    >
      <StatefulMenu
        items={actions}
        onItemSelect={({ item }: { item: ResolvedActionItem }) => {
          if (!item.disabled) onSelectAction(item);
        }}
        overrides={{
          Option: {
            component: ActionMenuItem,
            props: {
              onSelectAction,
              onClose,
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
