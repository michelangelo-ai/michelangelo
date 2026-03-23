import { renderHook } from '@testing-library/react';

import { createMockRow } from '../__fixtures__/mock-row';
import { DatetimeFilterValue } from '../datetime/types';
import { useDatetimeFilterFactory } from '../datetime/use-datetime-filter-factory';

// TODO(#977): — column param is partially unused in filter factories.
// isFilterInactive, getActiveFilter, and buildTableFilterFn do not depend on
// column identity; only getFilterSummary reads column.label. The factory API
// should make this explicit (e.g. accept label separately, or make column optional).

describe('Datetime Filter', () => {
  describe('Empty filter behavior', () => {
    it('shows all rows when no range or selection provided', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column is structurally unused in isFilterInactive
      const filterHook = result.current({});

      const emptyFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [],
        selection: [],
        description: '',
        exclude: false,
      };

      expect(filterHook.isFilterInactive(emptyFilter)).toBe(true);
    });

    it('considers filter active when range is provided', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column is structurally unused in isFilterInactive
      const filterHook = result.current({});

      const rangeFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterHook.isFilterInactive(rangeFilter)).toBe(false);
    });

    it('considers filter active when selection is provided', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column is structurally unused in isFilterInactive
      const filterHook = result.current({});

      const selectionFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [],
        selection: [1672531200], // 2023-01-01 epoch seconds
        description: 'Selected dates',
        exclude: false,
      };

      expect(filterHook.isFilterInactive(selectionFilter)).toBe(false);
    });
  });

  describe('Filter Display Functions', () => {
    it('returns empty string for inactive filters', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column is structurally unused when filter is inactive
      const filterHook = result.current({});

      const inactiveFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [],
        selection: [],
        description: '',
        exclude: false,
      };

      expect(filterHook.getActiveFilter(inactiveFilter)).toBe('');
      expect(filterHook.getFilterSummary(inactiveFilter)).toBe('');
    });

    it('returns description for active filters', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      const filterHook = result.current({
        id: 'createdAt',
        label: 'Created At',
        accessor: 'createdAt',
      });

      const activeFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterHook.getActiveFilter(activeFilter)).toBe('Year 2023');
      expect(filterHook.getFilterSummary(activeFilter)).toBe('Created At: Year 2023');
    });

    it('handles column without label in filter summary', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      const filterHook = result.current({ id: 'createdAt', accessor: 'createdAt' });

      const activeFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterHook.getFilterSummary(activeFilter)).toBe('Year 2023');
    });
  });

  describe('Filter Function Behavior', () => {
    it('returns true for inactive filters (show all rows)', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: number }>());
      // @ts-expect-error (#977) column is structurally unused when filter is inactive
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const mockRow = createMockRow({ createdAt: 1672531200 }); // 2023-01-01

      const inactiveFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [],
        selection: [],
        description: '',
        exclude: false,
      };

      expect(filterFn(mockRow, 'createdAt', inactiveFilter)).toBe(true);
    });

    it('filters rows based on date range (epoch seconds)', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: number }>());
      // @ts-expect-error (#977) column.id falls back to columnId param in getCellValueForColumn
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const jan1Row = createMockRow({ createdAt: 1672531200 }); // 2023-01-01
      const dec31Row = createMockRow({ createdAt: 1703980800 }); // 2023-12-31
      const beforeRangeRow = createMockRow({ createdAt: 1640995200 }); // 2022-01-01

      const filterValue: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterFn(jan1Row, 'createdAt', filterValue)).toBe(true);
      expect(filterFn(dec31Row, 'createdAt', filterValue)).toBe(true);
      expect(filterFn(beforeRangeRow, 'createdAt', filterValue)).toBe(false);
    });

    it('filters rows based on date range (string epoch seconds)', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: string }>());
      // @ts-expect-error (#977) column.id falls back to columnId param in getCellValueForColumn
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const validRow = createMockRow({ createdAt: '1672531200' }); // 2023-01-01
      const invalidRow = createMockRow({ createdAt: '1640995200' }); // 2022-01-01

      const filterValue: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterFn(validRow, 'createdAt', filterValue)).toBe(true);
      expect(filterFn(invalidRow, 'createdAt', filterValue)).toBe(false);
    });

    it('should not filter rows with null/undefined cell values', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column.id falls back to columnId param in getCellValueForColumn
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const nullRow = createMockRow({ createdAt: null });
      const undefinedRow = createMockRow({ createdAt: undefined });

      const filterValue: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      // @ts-expect-error testing null/undefined edge cases
      expect(filterFn(nullRow, 'createdAt', filterValue)).toBe(false);
      // @ts-expect-error testing null/undefined edge cases
      expect(filterFn(undefinedRow, 'createdAt', filterValue)).toBe(false);
    });

    it('should not filter rows with invalid date values', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: string }>());
      // @ts-expect-error (#977) column.id falls back to columnId param in getCellValueForColumn
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const invalidDateRow = createMockRow({ createdAt: 'invalid-number' });

      const filterValue: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterFn(invalidDateRow, 'createdAt', filterValue)).toBe(false);
    });

    it('handles string dates stored in localStorage', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: number }>());
      // @ts-expect-error (#977) column.id falls back to columnId param in getCellValueForColumn
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const validRow = createMockRow({ createdAt: 1672531200 }); // 2023-01-01

      const filterValue: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        // Simulate dates stored as strings in localStorage
        range: ['2023-01-01T00:00:00.000Z', '2023-12-31T23:59:59.000Z'] as unknown as Date[],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterFn(validRow, 'createdAt', filterValue)).toBe(true);
    });

    it('shows all rows when date range is incomplete (defensive behavior)', () => {
      // When start or end date is missing, the filter returns true for all rows
      // This is defensive behavior to avoid hiding data due to malformed filters
      const { result } = renderHook(() => useDatetimeFilterFactory<{ createdAt: number }>());
      // @ts-expect-error (#977) column is structurally unused when filter range is incomplete
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const mockRow = createMockRow({ createdAt: 1672531200 }); // 2023-01-01

      const incompleteFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01')], // Missing end date
        selection: [],
        description: 'Incomplete range',
        exclude: false,
      };

      expect(filterFn(mockRow, 'createdAt', incompleteFilter)).toBe(true);
    });

    it('has autoRemove property set to isFilterInactive function', () => {
      const { result } = renderHook(() => useDatetimeFilterFactory());
      // @ts-expect-error (#977) column is structurally unused in isFilterInactive
      const filterHook = result.current({});
      const filterFn = filterHook.buildTableFilterFn();

      const inactiveFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [],
        selection: [],
        description: '',
        exclude: false,
      };

      const activeFilter: DatetimeFilterValue = {
        operation: 'RANGE_DATETIME',
        range: [new Date('2023-01-01'), new Date('2023-12-31')],
        selection: [],
        description: 'Year 2023',
        exclude: false,
      };

      expect(filterFn.autoRemove!(inactiveFilter)).toBe(true);
      expect(filterFn.autoRemove!(activeFilter)).toBe(false);
    });
  });
});
