import { Checkbox } from 'baseui/checkbox';

import type { SelectableCapability } from '../../types/column-types';

export function TableSelectionColumn({
  canSelect,
  isSelected,
  onToggleSelection,
}: SelectableCapability) {
  if (!canSelect) {
    return null;
  }

  const applyToggle = (e: React.ChangeEvent<HTMLInputElement>) => {
    e.stopPropagation();
    onToggleSelection(e.target.checked);
  };

  return (
    <Checkbox
      checked={isSelected}
      onChange={applyToggle}
      overrides={{
        Root: {
          style: {
            alignItems: 'center',
          },
        },
      }}
    />
  );
}
