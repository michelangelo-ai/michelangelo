import { Row } from '@tanstack/react-table';
import { renderHook } from '@testing-library/react';

import { useCategoricalFilterFactory } from '../categorical/use-categorical-filter-factory';

const MOCK_COLUMN = {
  id: 'department',
  label: 'Department',
  accessor: 'department',
};

describe('Categorical Filter', () => {
  describe('Empty filter behavior', () => {
    it('shows all rows when no values selected', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.isFilterInactive([])).toBe(true);
    });

    it('should consider empty array as inactive filter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.isFilterInactive([])).toBe(true);
    });

    it('should consider undefined as inactive filter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      // @ts-expect-error undefined is not a valid filter value
      expect(filterHook.isFilterInactive(undefined)).toBe(true);
    });

    it('should consider non-empty array as active filter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.isFilterInactive(['Engineering'])).toBe(false);
    });
  });

  describe('Filter Display Functions', () => {
    it('should return empty string for inactive filters in getActiveFilter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.getActiveFilter([])).toBe('');
    });

    it('should format single value in getActiveFilter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.getActiveFilter(['Engineering'])).toBe('Engineering');
    });

    it('should format multiple values in getActiveFilter', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.getActiveFilter(['Engineering', 'Marketing'])).toBe(
        'Engineering, Marketing'
      );
    });

    it('should return empty string for inactive filters in getFilterSummary', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.getFilterSummary([])).toBe('');
    });

    it('should format filter summary with count and label', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);

      expect(filterHook.getFilterSummary(['Engineering'])).toBe('(1) Department: Engineering');
      expect(filterHook.getFilterSummary(['Engineering', 'Marketing'])).toBe(
        '(2) Department: Engineering, Marketing'
      );
    });
  });

  describe('Filter Function Behavior', () => {
    it('should return true for inactive filters (show all rows)', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory<{ department: string }>());
      const filterHook = result.current(MOCK_COLUMN);
      const filterFn = filterHook.buildTableFilterFn();

      const mockRow = {
        getValue: () => 'Engineering',
      } as unknown as Row<{ department: string }>;

      expect(filterFn(mockRow, 'department', [])).toBe(true);
    });

    it('should filter rows based on included values', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory<{ department: string }>());
      const filterHook = result.current(MOCK_COLUMN);
      const filterFn = filterHook.buildTableFilterFn();

      const engineeringRow = {
        getValue: () => 'Engineering',
        original: { department: 'Engineering' },
      } as unknown as Row<{ department: string }>;

      const marketingRow = {
        getValue: () => 'Marketing',
        original: { department: 'Marketing' },
      } as unknown as Row<{ department: string }>;

      const filterValue = ['Engineering', 'Design'];

      expect(filterFn(engineeringRow, 'department', filterValue)).toBe(true);
      expect(filterFn(marketingRow, 'department', filterValue)).toBe(false);
    });

    it('should have autoRemove property set to isFilterInactive function', () => {
      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(MOCK_COLUMN);
      const filterFn = filterHook.buildTableFilterFn();

      expect(filterFn.autoRemove!([])).toBe(true);
      expect(filterFn.autoRemove!(['Engineering'])).toBe(false);
    });
  });

  describe('Column Configuration Handling', () => {
    it('should use column label in filter summary', () => {
      const customColumn = {
        id: 'status',
        label: 'Employee Status',
        accessor: 'status',
      };

      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(customColumn);

      expect(filterHook.getFilterSummary(['Active'])).toBe('(1) Employee Status: Active');
    });

    it('should work with different column configurations', () => {
      const multiColumn = {
        id: 'name',
        label: 'Full Name',
        accessor: 'user.name',
        items: [
          { id: 'firstName', accessor: 'user.firstName' },
          { id: 'lastName', accessor: 'user.lastName' },
        ],
      };

      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(multiColumn);

      expect(filterHook.getFilterSummary(['John Doe'])).toBe('(1) Full Name: John Doe');
    });

    it('should handle omitted column label', () => {
      const multiColumn = {
        id: 'name',
        accessor: 'user.name',
        items: [
          { id: 'firstName', accessor: 'user.firstName' },
          { id: 'lastName', accessor: 'user.lastName' },
        ],
      };

      const { result } = renderHook(() => useCategoricalFilterFactory());
      const filterHook = result.current(multiColumn);

      expect(filterHook.getFilterSummary(['John Doe'])).toBe('(1) John Doe');
    });
  });
});
