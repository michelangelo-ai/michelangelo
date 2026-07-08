import { Icon } from '#core/components/icon/icon';
import { FilterOptionItem } from './styled-components';

import type { TableFilterOptionProps } from './types';

export function TableFilterOption({ label, onClick }: TableFilterOptionProps) {
  return (
    <FilterOptionItem
      onClick={onClick}
      role="option"
      aria-label={label}
      data-testid={`filter-option-${label}`}
    >
      {label}
      <Icon name="chevronRight" />
    </FilterOptionItem>
  );
}
