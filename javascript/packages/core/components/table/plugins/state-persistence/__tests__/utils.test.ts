import { getAllTableUserSettings, getTableUserSettings, updateUserTableSettings } from '../utils';

import type { TableState } from '#core/components/table/types/table-types';

describe('state-persistence utils', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('getAllTableUserSettings', () => {
    it('returns localStorage data when available', () => {
      const mockData = {
        'table-1': { globalFilter: 'search-1' },
        'table-2': { globalFilter: 'search-2' },
      };
      localStorage.setItem('ma-studio-table-settings', JSON.stringify(mockData));

      const result = getAllTableUserSettings();

      expect(result).toEqual(mockData);
    });

    it('returns empty object when localStorage is empty', () => {
      const result = getAllTableUserSettings();

      expect(result).toEqual({});
    });
  });

  describe('getTableUserSettings', () => {
    it('returns settings for specific table ID', () => {
      const mockData = {
        'table-1': { globalFilter: 'search-1' },
        'table-2': { globalFilter: 'search-2' },
      };
      localStorage.setItem('ma-studio-table-settings', JSON.stringify(mockData));

      const result = getTableUserSettings('table-1');

      expect(result).toEqual({ globalFilter: 'search-1' });
    });

    it('returns empty object when table ID not found', () => {
      localStorage.setItem('ma-studio-table-settings', JSON.stringify({}));

      const result = getTableUserSettings('non-existent-table');

      expect(result).toEqual({});
    });

    it('returns empty object when no table ID provided', () => {
      const result = getTableUserSettings();

      expect(result).toEqual({});
    });
  });

  describe('updateUserTableSettings', () => {
    it('saves new settings using dot notation and returns true', () => {
      const result = updateUserTableSettings('table-1.globalFilter', 'new-search');

      expect(result).toBe(true);

      const stored = JSON.parse(localStorage.getItem('ma-studio-table-settings')!) as Record<
        string,
        Partial<TableState>
      >;
      expect(stored).toEqual({ 'table-1': { globalFilter: 'new-search' } });
    });

    it('returns false when new settings are identical to existing', () => {
      updateUserTableSettings('table-1.globalFilter', 'existing-search');

      const result = updateUserTableSettings('table-1.globalFilter', 'existing-search');

      expect(result).toBe(false);
    });

    it('merges new settings with existing data', () => {
      updateUserTableSettings('table-1.globalFilter', 'existing');
      updateUserTableSettings('table-2.globalFilter', 'other-table');

      updateUserTableSettings('table-1.globalFilter', 'updated');

      const stored = JSON.parse(localStorage.getItem('ma-studio-table-settings')!) as Record<
        string,
        Partial<TableState>
      >;
      expect(stored).toEqual({
        'table-1': { globalFilter: 'updated' },
        'table-2': { globalFilter: 'other-table' },
      });
    });
  });
});
