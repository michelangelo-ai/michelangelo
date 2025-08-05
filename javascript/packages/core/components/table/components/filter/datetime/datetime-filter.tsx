import { DatetimeColumn } from 'baseui/data-table';

import { UNIFIED_API_ORIGIN_DATE } from './constants';
import { DatetimeFilterValue } from './types';
import { convertStringParamsToDate } from './utils';

import type { ColumnFilterProps } from '../types';

export function DatetimeFilter({ close, getFilterValue, setFilterValue }: ColumnFilterProps) {
  // BaseUI requires these props but we don't use them in filter context
  const DatetimeFilterPanel = DatetimeColumn({
    title: '',
    mapDataToValue: () => new Date(),
  }).renderFilter;

  const filterRange = [UNIFIED_API_ORIGIN_DATE, new Date()];
  const currentFilterValue = convertStringParamsToDate(getFilterValue() as DatetimeFilterValue);

  return (
    <DatetimeFilterPanel
      data={filterRange}
      setFilter={setFilterValue as (value: DatetimeFilterValue) => void}
      close={close}
      // @ts-expect-error Michelangelo DatetimeFilterValue does not match BaseUI's FilterParameters type
      filterParams={currentFilterValue}
    />
  );
}
