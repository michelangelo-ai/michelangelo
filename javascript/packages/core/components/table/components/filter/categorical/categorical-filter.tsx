import { CategoricalColumn } from 'baseui/data-table';

import { safeStringify } from '#core/utils/string-utils';
import { useFilterDisplayMapping } from './use-filter-display-mapping';

import type { ColumnFilterProps } from '../types';

export function CategoricalFilter<TData = unknown>({
  column,
  close,
  getFilterValue,
  setFilterValue,
  preFilteredRows,
}: ColumnFilterProps<TData>) {
  // BaseUI requires these props but we don't use them in filter context
  const CategoricalFilterPanel = CategoricalColumn({
    title: '',
    mapDataToValue: () => '',
  }).renderFilter;

  const [filterValueToDisplayValue, displayValueToFilterValue] = useFilterDisplayMapping({
    preFilteredRows,
    column,
  });

  const availableDisplayValues = Object.keys(displayValueToFilterValue);
  const currentDisplaySelection = ((getFilterValue() as unknown[]) ?? [])
    .map((value) => filterValueToDisplayValue[safeStringify(value)])
    .filter(Boolean);

  // Sort display values: selected items first, then alphabetical within each group
  const sortedDisplayValues = availableDisplayValues.sort((a, b) => {
    const isSelectedA = currentDisplaySelection.includes(a);
    const isSelectedB = currentDisplaySelection.includes(b);

    if (isSelectedA === isSelectedB) {
      return a.localeCompare(b);
    }
    return isSelectedA ? -1 : 1;
  });

  const handleFilterChange = ({
    selection,
    exclude,
  }: {
    selection: Set<string>;
    exclude: boolean;
  }) => {
    // Apply exclude logic: invert selection if exclude is true
    const selectedDisplayValues = exclude
      ? availableDisplayValues.filter((displayValue) => !selection.has(displayValue))
      : Array.from(selection);

    const filterValues = selectedDisplayValues.map(
      (displayValue) => displayValueToFilterValue[displayValue]
    );

    setFilterValue(filterValues.length > 0 ? filterValues : undefined);
    close();
  };

  return (
    <CategoricalFilterPanel
      data={sortedDisplayValues}
      setFilter={handleFilterChange}
      close={close}
      filterParams={{
        description: '',
        selection: new Set(currentDisplaySelection),
        exclude: false,
      }}
    />
  );
}
