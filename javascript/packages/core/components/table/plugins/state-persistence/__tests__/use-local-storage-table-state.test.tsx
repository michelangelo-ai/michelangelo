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
  const buildTestData = () => [
    { id: '1', name: 'Alice Johnson', department: 'Engineering', status: 'Active' },
    { id: '2', name: 'Bob Smith', department: 'Marketing', status: 'Inactive' },
    { id: '3', name: 'Carol Davis', department: 'Engineering', status: 'Active' },
    { id: '4', name: 'David Wilson', department: 'Sales', status: 'Active' },
  ];

  const buildTestColumns = () => [
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

    it('persists filter settings under filterSettingsId when provided', () => {
      const { result } = renderHook(() =>
        useLocalStorageTableState({
          tableSettingsId: 'test-table',
          filterSettingsId: 'test-table.project-123',
        })
      );

      act(() => {
        result.current.setGlobalFilter('project-filter');
        result.current.setColumnFilters([{ id: 'status', value: ['Active'] }]);
        result.current.setPagination({ pageIndex: 0, pageSize: 25 });
      });

      const storedData = localStorage.getItem(TABLE_LOCAL_STORAGE_KEY);
      expect(storedData).toBeTruthy();

      const parsedData = JSON.parse(storedData!) as
        | Record<string, Partial<TableState>>
        | Record<string, Record<string, Partial<TableState>>>;

      // Filters stored under filterSettingsId
      expect((parsedData['test-table']['project-123'] as Partial<TableState>).globalFilter).toBe(
        'project-filter'
      );
      expect(
        (parsedData['test-table']['project-123'] as Partial<TableState>).columnFilters
      ).toEqual([{ id: 'status', value: ['Active'] }]);

      // Global settings stored under tableSettingsId
      expect((parsedData['test-table'] as TableState['pagination']).pageSize).toEqual(25);
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

    it('restores project-specific filter state when filterSettingsId is provided', () => {
      const existingState = {
        'test-table.project-123.globalFilter': 'project-restored-filter',
        'test-table.project-123.columnFilters': [{ id: 'department', value: ['Sales'] }],
        'test-table.columnVisibility': { status: false },
        'test-table.pageSize': 15,
      };
      localStorage.setItem(TABLE_LOCAL_STORAGE_KEY, JSON.stringify(existingState));

      const { result } = renderHook(() =>
        useLocalStorageTableState({
          tableSettingsId: 'test-table',
          filterSettingsId: 'test-table.project-123',
        })
      );

      expect(result.current.globalFilter).toBe('project-restored-filter');
      expect(result.current.columnFilters).toEqual([{ id: 'department', value: ['Sales'] }]);
      expect(result.current.columnVisibility).toEqual({ status: false });
      expect(result.current.pagination.pageSize).toBe(15);
    });

    it('uses initial state when no persisted data exists', () => {
      const { result } = renderHook(() =>
        useLocalStorageTableState({
          tableSettingsId: 'test-table',
          initialState: {
            globalFilter: 'initial-filter',
            columnVisibility: { name: false },
          },
        })
      );

      expect(result.current.globalFilter).toBe('initial-filter');
      expect(result.current.columnVisibility).toEqual({ name: false });
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

    it('allows different projects to have separate filter state for same table', () => {
      const { result: project1 } = renderHook(() =>
        useLocalStorageTableState({
          tableSettingsId: 'user-table',
          filterSettingsId: 'user-table.project-123',
        })
      );

      const { result: project2 } = renderHook(() =>
        useLocalStorageTableState({
          tableSettingsId: 'user-table',
          filterSettingsId: 'user-table.project-456',
        })
      );

      act(() => {
        project1.current.setGlobalFilter('project-123-filter');
        project1.current.setColumnFilters([{ id: 'status', value: ['Active'] }]);
        project1.current.setColumnVisibility({ name: false });

        project2.current.setGlobalFilter('project-456-filter');
        project2.current.setColumnFilters([{ id: 'status', value: ['Inactive'] }]);
        project2.current.setColumnVisibility({ department: false });
      });

      const storedData = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as
        | Record<string, Partial<TableState>>
        | Record<string, Record<string, Partial<TableState>>>;

      // Project 1 filters stored separately
      expect((storedData['user-table']['project-123'] as Partial<TableState>).globalFilter).toBe(
        'project-123-filter'
      );
      expect(
        (storedData['user-table']['project-123'] as Partial<TableState>).columnFilters
      ).toEqual([{ id: 'status', value: ['Active'] }]);

      // Project 2 filters stored separately
      expect((storedData['user-table']['project-456'] as Partial<TableState>).globalFilter).toBe(
        'project-456-filter'
      );
      expect(
        (storedData['user-table']['project-456'] as Partial<TableState>).columnFilters
      ).toEqual([{ id: 'status', value: ['Inactive'] }]);

      // Global settings should be shared (last write wins)
      expect((storedData['user-table'] as Partial<TableState>).columnVisibility).toEqual({
        department: false,
      });
    });
  });

  describe('integration with Table component', () => {
    function TableWithPersistence({ tableSettingsId }: { tableSettingsId: string }) {
      const tableState = useLocalStorageTableState({ tableSettingsId });

      return (
        <Table
          data={buildTestData()}
          columns={buildTestColumns()}
          state={tableState}
          actionBarConfig={{ enableSearch: true }}
        />
      );
    }

    function TableWithProjectFilters({
      tableSettingsId,
      filterSettingsId,
    }: {
      tableSettingsId: string;
      filterSettingsId?: string;
    }) {
      const tableState = useLocalStorageTableState({ tableSettingsId, filterSettingsId });

      return (
        <Table
          data={buildTestData()}
          columns={buildTestColumns()}
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

    it('isolates filter state between projects while sharing global settings', async () => {
      const user = userEvent.setup({
        advanceTimers: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
      });

      const { unmount: unmountProject1 } = render(
        <TableWithProjectFilters
          tableSettingsId="shared-table"
          filterSettingsId="shared-table.project-1"
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      await user.type(screen.getByRole('searchbox'), 'Engineering');
      vi.runAllTimers();

      await waitFor(() => {
        expect(screen.getAllByRole('row')).toHaveLength(3); // 1 header + 2 Engineering rows
      });

      unmountProject1();

      render(
        <TableWithProjectFilters
          tableSettingsId="shared-table"
          filterSettingsId="shared-table.project-2"
        />,
        buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
      );

      expect(screen.getByRole('searchbox')).toHaveValue('');
      expect(screen.getAllByRole('row')).toHaveLength(5);

      const storedData = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as Record<
        string,
        Partial<TableState> | Record<string, Partial<TableState>>
      >;

      expect((storedData['shared-table']['project-1'] as Partial<TableState>).globalFilter).toBe(
        'Engineering'
      );
      expect(storedData['shared-table']['project-2']).toBeUndefined();
    });
  });
});
