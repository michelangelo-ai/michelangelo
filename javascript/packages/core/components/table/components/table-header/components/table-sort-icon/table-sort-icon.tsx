import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';

import type { TableSortIconProps } from './types';

export function TableSortIcon(props: TableSortIconProps) {
  const { column } = props;

  const sortDirection = column.getIsSorted();
  const iconName = sortDirection === 'desc' ? 'sortDescending' : 'sortAscending';
  const iconKind = sortDirection ? IconKind.ACCENT : IconKind.TERTIARY;

  return <Icon name={iconName} kind={iconKind} title="Toggle Sort" size={16} />;
}
