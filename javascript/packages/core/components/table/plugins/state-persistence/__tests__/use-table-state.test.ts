import { act, renderHook } from '@testing-library/react';
import { vi } from 'vitest';

import { TABLE_LOCAL_STORAGE_KEY } from '#core/components/table/constants';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { useTableState } from '../use-table-state';

describe('useTableState', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  describe('without urlFilters', () => {
    const VALID_COLUMN_IDS = ['name', 'department', 'status'];

    it('always exposes buildShareUrl as a callable function', () => {
      const { result } = renderHook(
        () => useTableState({ tableSettingsId: 'test-table' }),
        buildWrapper([getRouterWrapper()])
      );
      expect(typeof result.current.buildShareUrl).toBe('function');
    });

    it('reads and persists state via localStorage', () => {
      const { result } = renderHook(
        () => useTableState({ tableSettingsId: 'test-table' }),
        buildWrapper([getRouterWrapper()])
      );

      act(() => {
        result.current.setGlobalFilter('hello');
      });

      expect(result.current.globalFilter).toBe('hello');
      const stored = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as Record<
        string,
        Record<string, string>
      >;
      expect(stored['test-table'].globalFilter).toBe('hello');
    });

    it('ignores URL params when urlFilters is not enabled', () => {
      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.gf=fromurl' })])
      );
      expect(result.current.globalFilter).toBe('');
    });
  });

  describe('with urlFilters.enabled = true', () => {
    const VALID_COLUMN_IDS = ['name', 'department', 'status'];

    it('bypasses localStorage and reads from URL when URL params are present', () => {
      localStorage.setItem(
        TABLE_LOCAL_STORAGE_KEY,
        JSON.stringify({ 'test-table.globalFilter': 'fromlocal' })
      );

      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.gf=fromurl' })])
      );

      expect(result.current.globalFilter).toBe('fromurl');
    });

    it('bypasses initialState and reads from URL when URL params are present', () => {
      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            initialState: { globalFilter: 'frominitial' },
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.gf=fromurl' })])
      );

      expect(result.current.globalFilter).toBe('fromurl');
    });

    it('uses localStorage when URL has no matching params', () => {
      localStorage.setItem(
        TABLE_LOCAL_STORAGE_KEY,
        JSON.stringify({ 'test-table.globalFilter': 'fromlocal' })
      );

      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper()])
      );

      expect(result.current.globalFilter).toBe('fromlocal');
    });

    it('silently drops invalid column IDs from URL params', () => {
      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([
          getRouterWrapper({
            location: '/?tb.test-table.cf=ghost:eq:x,status:eq:active',
          }),
        ])
      );

      expect(result.current.columnFilters).toEqual([{ id: 'status', value: 'active' }]);
    });

    it('reads sorting from URL', () => {
      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.so=name:desc' })])
      );

      expect(result.current.sorting).toEqual([{ id: 'name', desc: true }]);
    });

    it('setGlobalFilter updates in-memory state without writing to localStorage', () => {
      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.gf=fromurl' })])
      );

      expect(result.current.globalFilter).toBe('fromurl');

      act(() => {
        result.current.setGlobalFilter('changed');
      });

      expect(result.current.globalFilter).toBe('changed');
      expect(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)).toBeNull();
    });

    it('in-memory state initialised from URL is independent of localStorage', () => {
      localStorage.setItem(
        TABLE_LOCAL_STORAGE_KEY,
        JSON.stringify({ 'test-table': { globalFilter: 'fromlocal' } })
      );

      const { result } = renderHook(
        () =>
          useTableState({
            tableSettingsId: 'test-table',
            validColumnIds: VALID_COLUMN_IDS,
            urlFilters: { enabled: true },
          }),
        buildWrapper([getRouterWrapper({ location: '/?tb.test-table.gf=fromurl' })])
      );

      // URL takes precedence over localStorage on mount
      expect(result.current.globalFilter).toBe('fromurl');

      act(() => {
        result.current.setGlobalFilter('usertyped');
      });

      // localStorage remains unchanged
      const stored = JSON.parse(localStorage.getItem(TABLE_LOCAL_STORAGE_KEY)!) as Record<
        string,
        Record<string, string>
      >;
      expect(stored['test-table'].globalFilter).toBe('fromlocal');
      expect(result.current.globalFilter).toBe('usertyped');
    });
  });
});
