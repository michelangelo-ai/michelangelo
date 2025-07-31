import { act, renderHook } from '@testing-library/react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { TABLE_LOCAL_STORAGE_KEY } from '#core/components/table/constants';
import { Table } from '#core/components/table/table';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { useLocalStorageTableState } from '../use-local-storage-table-state';

import type { TableState } from '#core/components/table/types/table-types';

describe('useLocalStorageTableState', () => {
  const testData = [
    { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
    { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
    { id: '3', name: 'Carol Davis', department: 'Engineering', status: 'Active' },
    { id: '4', name: 'David Wilson', department: 'Sales', status: 'Active' },
  ];

  const testColumns = [
    { id: 'name', label: 'Name' },
    { id: 'department', label: 'Department' },
    { id: 'status', label: 'Status' },
  ];

  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.runOnlyPendingTimers();
    vi.useRealTimers();
  });

  describe('hook behavior', () => {
    it('returns default state when no persisted data exists', () => {
      const { result } = renderHook(() =>
        useLocalStorageTableState({ tableSettingsId: 'test-table' })
      );

      expect(result.current.globalFilter).toBe('');
      expect(typeof result.current.setGlobalFilter).toBe('function');
    });

    it('persists state changes to localStorage', () => {
      const { result } = renderHook(() =>
        useLocalStorageTableState({ tableSettingsId: 'test-table' })
      );

      act(() => {
        result.current.setGlobalFilter('test-filter');
      });

      const storedData = localStorage.getItem(TABLE_LOCAL_STORAGE_KEY);
      expect(storedData).toBeTruthy();

      const parsedData = JSON.parse(storedData!) as Record<string, Partial<TableState>>;
      expect(parsedData['test-table'].globalFilter).toBe('test-filter');
    });

    it('restores state from localStorage on initialization', () => {
      const existingState = {
        'test-table.globalFilter': 'restored-filter',
      };
      localStorage.setItem(TABLE_LOCAL_STORAGE_KEY, JSON.stringify(existingState));

      const { result } = renderHook(() =>
        useLocalStorageTableState({ tableSettingsId: 'test-table' })
      );

      expect(result.current.globalFilter).toBe('restored-filter');
    });

    it('handles localStorage errors gracefully', () => {
      const originalGetItem = localStorage.getItem.bind(localStorage) as unknown as () => string;
      localStorage.getItem = vi.fn(() => {
        throw new Error('SecurityError');
      });

      expect(() => {
        renderHook(() => useLocalStorageTableState({ tableSettingsId: 'test-table' }));
      }).not.toThrow();

      localStorage.getItem = originalGetItem;
    });

    it('maintains separate state for different table settings IDs', () => {
      const { result: result1 } = renderHook(() =>
        useLocalStorageTableState({ tableSettingsId: 'table-1' })
      );

      const { result: result2 } = renderHook(() =>
        useLocalStorageTableState({ tableSettingsId: 'table-2' })
      );

      act(() => {
        result1.current.setGlobalFilter('filter-1');
        result2.current.setGlobalFilter('filter-2');
      });

      const storedData = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as Record<
        string,
        Partial<TableState>
      >;
      expect(storedData['table-1'].globalFilter).toBe('filter-1');
      expect(storedData['table-2'].globalFilter).toBe('filter-2');
    });
  });

  describe('integration with Table component', () => {
    function TableWithPersistence({ tableSettingsId }: { tableSettingsId: string }) {
      const tableState = useLocalStorageTableState({ tableSettingsId });

      return (
        <Table
          data={testData}
          columns={testColumns}
          state={tableState}
          actionBarConfig={{ enableSearch: true }}
        />
      );
    }

    it('persists search state through table interactions', async () => {
      const tableSettingsId = 'integration-test';

      render(
        <TableWithPersistence tableSettingsId={tableSettingsId} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      const user = userEvent.setup({
        advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
      });

      await user.type(screen.getByRole('searchbox'), 'Engineering');
      vi.runAllTimers();

      await waitFor(() => {
        expect(screen.getAllByRole('row')).toHaveLength(3); // 1 header + 2 Engineering rows
      });

      await waitFor(() => {
        const storedData = localStorage.getItem(TABLE_LOCAL_STORAGE_KEY);
        expect(storedData).toBeTruthy();

        const parsedData = JSON.parse(storedData!) as Record<string, TableState>;
        expect(parsedData[tableSettingsId].globalFilter).toEqual('Engineering');
      });
    });

    it('restores search state when component remounts', () => {
      const tableSettingsId = 'remount-test';
      const existingState = {
        [`${tableSettingsId}.globalFilter`]: 'Marketing',
      };
      localStorage.setItem(TABLE_LOCAL_STORAGE_KEY, JSON.stringify(existingState));

      render(
        <TableWithPersistence tableSettingsId={tableSettingsId} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByRole('searchbox')).toHaveValue('Marketing');
      expect(screen.getAllByRole('row')).toHaveLength(2); // 1 header + 1 Marketing row
    });

    it('allows clearing persisted search state', async () => {
      const tableSettingsId = 'clear-test';
      const existingState = {
        [tableSettingsId]: {
          globalFilter: 'Engineering',
        },
      };
      localStorage.setItem(TABLE_LOCAL_STORAGE_KEY, JSON.stringify(existingState));

      render(
        <TableWithPersistence tableSettingsId={tableSettingsId} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByRole('searchbox')).toHaveValue('Engineering');
      expect(screen.getAllByRole('row')).toHaveLength(3);

      const user = userEvent.setup({
        advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
      });

      await user.click(screen.getByLabelText('Clear value'));
      vi.runAllTimers();

      await waitFor(() => {
        expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 data rows
      });

      await waitFor(() => {
        const storedData = localStorage.getItem(TABLE_LOCAL_STORAGE_KEY);
        const parsedData = JSON.parse(storedData!) as Record<string, Partial<TableState>>;
        expect(parsedData[tableSettingsId].globalFilter).toBe('');
      });
    });

    it('handles multiple tables with different settings IDs', async () => {
      const tableSettingsId1 = 'multi-table-1';
      const tableSettingsId2 = 'multi-table-2';

      const { unmount } = render(
        <TableWithPersistence tableSettingsId={tableSettingsId1} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      const user = userEvent.setup({
        advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
      });

      await user.type(screen.getByRole('searchbox'), 'Engineering');
      vi.runAllTimers();

      await waitFor(() => {
        expect(screen.getAllByRole('row')).toHaveLength(3);
      });

      unmount();

      render(
        <TableWithPersistence tableSettingsId={tableSettingsId2} />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByRole('searchbox')).toHaveValue('');
      expect(screen.getAllByRole('row')).toHaveLength(5); // 1 header + 4 data rows

      const storedData = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as Record<
        string,
        Partial<TableState>
      >;
      expect(storedData[tableSettingsId1].globalFilter).toBe('Engineering');
      expect(storedData[tableSettingsId2]).toBeUndefined();
    });
  });
});
