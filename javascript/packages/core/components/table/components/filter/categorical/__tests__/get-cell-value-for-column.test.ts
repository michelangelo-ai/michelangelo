import { getCellValueForColumn } from '#core/components/table/components/filter/categorical/get-cell-value-for-column';

import type { Row } from '@tanstack/react-table';
import type { FilterableRow } from '#core/components/table/components/filter/types';
import type { ColumnConfig } from '#core/components/table/types/column-types';

describe('getCellValueForColumn', () => {
  const mockRecord = {
    name: 'my-pipeline',
    version: 'v1.0',
    status: 'running',
  };

  const createMockRow = (getValue: (id: string) => unknown): Row<unknown> =>
    ({
      getValue,
      original: mockRecord,
    }) as Row<unknown>;

  const createMockFilterableRow = (getValue: (id: string) => unknown): FilterableRow<unknown> => ({
    getValue,
    record: mockRecord,
  });

  describe('regular columns (no items)', () => {
    it('should return raw value for regular column with TanStack Row', () => {
      const column: ColumnConfig = {
        id: 'name',
        label: 'Name',
      };

      const row = createMockRow(() => 'test-value');
      const result = getCellValueForColumn(column, row, 'name');

      expect(result).toBe('test-value');
    });

    it('should return raw value for regular column with FilterableRow', () => {
      const column: ColumnConfig = {
        id: 'name',
        label: 'Name',
      };

      const row = createMockFilterableRow(() => 'test-value');
      const result = getCellValueForColumn(column, row, 'name');

      expect(result).toBe('test-value');
    });

    it('should handle null/undefined values for regular columns', () => {
      const column: ColumnConfig = {
        id: 'name',
        label: 'Name',
      };

      const row = createMockRow(() => null);
      const result = getCellValueForColumn(column, row, 'name');

      expect(result).toBe('');
    });

    it('should return objects unchanged for regular columns', () => {
      const column: ColumnConfig = {
        id: 'metadata',
        label: 'Metadata',
      };

      const complexObject = { nested: { value: 'test' } };
      const row = createMockRow(() => complexObject);
      const result = getCellValueForColumn(column, row, 'metadata');

      expect(result).toBe(complexObject);
    });
  });

  describe('multi-cell columns (with items)', () => {
    const multiCellColumn: ColumnConfig = {
      id: 'pipeline-info',
      label: 'Pipeline Info',
      items: [
        { id: 'name', accessor: 'name' },
        { id: 'version', accessor: 'version' },
        { id: 'status', accessor: 'status' },
      ],
    };

    it('should extract first item from joined string with TanStack Row', () => {
      const joinedValue = 'my-pipeline__JOIN__v1.0__JOIN__running';
      const row = createMockRow(() => joinedValue);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('my-pipeline');
    });

    it('should extract first item from joined string with FilterableRow', () => {
      const joinedValue = 'data-processor__JOIN__v2.1__JOIN__stopped';
      const row = createMockFilterableRow(() => joinedValue);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('data-processor');
    });

    it('should handle empty first item in joined string', () => {
      const joinedValue = '__JOIN__v1.0__JOIN__running';
      const row = createMockRow(() => joinedValue);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('');
    });

    it('should handle single item without join string', () => {
      const row = createMockRow(() => 'single-value');

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('single-value');
    });

    it('should warn and fallback when multi-cell column returns non-string', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => null);
      const objectValue = { name: 'my-pipeline', version: 'v1.0' };
      const row = createMockRow(() => objectValue);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(consoleSpy).toHaveBeenCalledWith(
        'Expected string from normalizeColumnAccessor for multi-cell column pipeline-info, got:',
        'object',
        objectValue
      );
      expect(result).toBe('{"name":"my-pipeline","version":"v1.0"}'); // safeStringify converts object to JSON, no __JOIN__ to split

      consoleSpy.mockRestore();
    });

    it('should warn and fallback when multi-cell column returns number', () => {
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => null);
      const row = createMockRow(() => 42);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(consoleSpy).toHaveBeenCalledWith(
        'Expected string from normalizeColumnAccessor for multi-cell column pipeline-info, got:',
        'number',
        42
      );
      expect(result).toBe('42'); // safeStringify converts to string, no __JOIN__ to split

      consoleSpy.mockRestore();
    });

    it('should handle null/undefined for multi-cell columns', () => {
      const row = createMockRow(() => null);

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('');
    });

    it('should use effective column id when resolveColumnForRow returns different id', () => {
      // This tests the resolveColumnForRow integration
      const joinedValue = 'resolved-pipeline__JOIN__v3.0__JOIN__active';
      const row = createMockRow((id) => {
        if (id === 'pipeline-info') return joinedValue;
        return undefined;
      });

      const result = getCellValueForColumn(multiCellColumn, row, 'pipeline-info');

      expect(result).toBe('resolved-pipeline');
    });
  });

  describe('edge cases', () => {
    it('should handle empty string from getValue', () => {
      const column: ColumnConfig = {
        id: 'empty',
        label: 'Empty',
        items: [{ id: 'name' }],
      };

      const row = createMockRow(() => '');
      const result = getCellValueForColumn(column, row, 'empty');

      expect(result).toBe('');
    });

    it('should handle whitespace-only string for multi-cell', () => {
      const column: ColumnConfig = {
        id: 'whitespace',
        label: 'Whitespace',
        items: [{ id: 'name' }],
      };

      const row = createMockRow(() => '   ');
      const result = getCellValueForColumn(column, row, 'whitespace');

      expect(result).toBe('   '); // Should preserve whitespace
    });

    it('should handle complex joined values with special characters', () => {
      const column: ColumnConfig = {
        id: 'complex',
        label: 'Complex',
        items: [{ id: 'name' }, { id: 'description' }],
      };

      const joinedValue = 'pipeline-with-dashes__JOIN__Description with spaces and symbols!@#';
      const row = createMockRow(() => joinedValue);

      const result = getCellValueForColumn(column, row, 'complex');

      expect(result).toBe('pipeline-with-dashes');
    });
  });
});
