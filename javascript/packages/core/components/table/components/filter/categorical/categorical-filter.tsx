import { CategoricalColumn } from 'baseui/data-table';

import { safeStringify } from '#core/utils/string-utils';

import type { ColumnFilterProps } from '../types';

export function CategoricalFilter({
  columnId,
  close,
  getFilterValue,
  setFilterValue,
  preFilteredRows,
}: ColumnFilterProps) {
  // BaseUI requires these props but we don't use them in filter context
  const CategoricalFilterPanel = CategoricalColumn({
    title: '',
    mapDataToValue: () => '',
  }).renderFilter;

  const uniqueValues = new Set<string>();
  preFilteredRows.forEach((row) => {
    const value = row.getValue(columnId);
    if (value != null) {
      uniqueValues.add(safeStringify(value));
    }
  });
  const availableValues = Array.from(uniqueValues);

  const currentSelection = (getFilterValue() as string[]) ?? [];

  // Sort values: selected items first, then alphabetical within each group
  const sortedValues = availableValues.sort((a, b) => {
    const isSelectedA = currentSelection.includes(a);
    const isSelectedB = currentSelection.includes(b);

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
    const filteredSelection = exclude
      ? availableValues.filter((value) => !selection.has(value))
      : Array.from(selection);

    setFilterValue(filteredSelection.length > 0 ? filteredSelection : undefined);
    close();
  };

  return (
    <CategoricalFilterPanel
      data={sortedValues}
      setFilter={handleFilterChange}
      close={close}
      filterParams={{
        description: '',
        selection: new Set(currentSelection),
        exclude: false,
      }}
    />
  );
}
