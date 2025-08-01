import { Icon } from '#core/components/icon/icon';
import { FilterOptionItem } from './styled-components';

import type { TableFilterOptionProps } from './types';

export function TableFilterOption({ label, onClick }: TableFilterOptionProps) {
  return (
    <FilterOptionItem onClick={onClick}>
      {label}
      <Icon name="chevronRight" size={20} />
    </FilterOptionItem>
  );
}
