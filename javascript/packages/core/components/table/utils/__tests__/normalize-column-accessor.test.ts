import { normalizeColumnAccessor } from '../normalize-column-accessor';

import type { ColumnConfig } from '#core/components/table/types/column-types';

describe('normalizeColumnAccessor', () => {
  it('should create accessor function using column id when no accessor is provided', () => {
    const column: ColumnConfig = {
      id: 'name',
      label: 'Name',
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ name: 'John Doe', age: 30 });

    expect(result).toBe('John Doe');
  });

  it('should create accessor function using provided accessor string', () => {
    const column: ColumnConfig = {
      id: 'fullName',
      label: 'Full Name',
      accessor: 'user.name',
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ user: { name: 'Jane Smith' }, id: 1 });

    expect(result).toBe('Jane Smith');
  });

  it('should create accessor function using provided accessor function', () => {
    const column: ColumnConfig = {
      id: 'displayName',
      label: 'Display Name',
      accessor: (row: { firstName: string; lastName: string }) =>
        `${row.firstName} ${row.lastName}`,
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ firstName: 'John', lastName: 'Doe' });

    expect(result).toBe('John Doe');
  });

  it('should handle nested object paths', () => {
    const column: ColumnConfig = {
      id: 'address',
      label: 'Address',
      accessor: 'user.address.street',
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      user: {
        address: {
          street: '123 Main St',
          city: 'Anytown',
        },
      },
    });

    expect(result).toBe('123 Main St');
  });

  it('should return undefined for non-existent paths', () => {
    const column: ColumnConfig = {
      id: 'missing',
      label: 'Missing',
      accessor: 'non.existent.path',
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ name: 'John' });

    expect(result).toBeUndefined();
  });

  it('should join values from multiple items with MULTI_COLUMN_DATA_JOIN_STRING', () => {
    const column: ColumnConfig = {
      id: 'pipeline-info',
      label: 'Pipeline Info',
      items: [
        { id: 'name', accessor: 'name' },
        { id: 'version', accessor: 'version' },
        { id: 'status', accessor: 'status' },
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      name: 'my-pipeline',
      version: 'v1.0',
      status: 'running',
    });

    expect(result).toBe('my-pipeline__JOIN__v1.0__JOIN__running');
  });

  it('should handle missing values in multi-cell items', () => {
    const column: ColumnConfig = {
      id: 'info',
      label: 'Info',
      items: [
        { id: 'name', accessor: 'name' },
        { id: 'missing', accessor: 'missing' },
        { id: 'status', accessor: 'status' },
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      name: 'test',
      status: 'active',
    });

    expect(result).toBe('test__JOIN____JOIN__active');
  });

  it('should handle empty multi-cell items array', () => {
    const column: ColumnConfig = {
      id: 'empty',
      label: 'Empty',
      items: [],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ name: 'test' });

    expect(result).toBe('');
  });

  it('should use item accessor over item id when both are provided', () => {
    const column: ColumnConfig = {
      id: 'mixed',
      label: 'Mixed',
      items: [{ id: 'wrongPath', accessor: 'correctPath' }],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      wrongPath: 'wrong',
      correctPath: 'correct',
    });

    expect(result).toBe('correct');
  });

  it('should fall back to item id when accessor is not provided', () => {
    const column: ColumnConfig = {
      id: 'fallback',
      label: 'Fallback',
      items: [
        { id: 'name' }, // no accessor
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({ name: 'test-value' });

    expect(result).toBe('test-value');
  });

  it('should handle nested paths in multi-cell items', () => {
    const column: ColumnConfig = {
      id: 'nested',
      label: 'Nested',
      items: [
        { id: 'userProfile', accessor: 'user.profile.name' },
        { id: 'userEmail', accessor: 'user.email' },
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      user: {
        profile: { name: 'John Doe' },
        email: 'john@example.com',
      },
    });

    expect(result).toBe('John Doe__JOIN__john@example.com');
  });

  it('should handle mixed primitive and complex values in multi-cell items', () => {
    const column: ColumnConfig = {
      id: 'mixed-types',
      label: 'Mixed Types',
      items: [
        { id: 'name', accessor: 'name' },
        { id: 'count', accessor: 'count' },
        { id: 'active', accessor: 'active' },
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      name: 'test',
      count: 42,
      active: true,
    });

    expect(result).toBe('test__JOIN__42__JOIN__true');
  });

  it('should handle null and undefined values in multi-cell items', () => {
    const column: ColumnConfig = {
      id: 'nullish',
      label: 'Nullish',
      items: [
        { id: 'name', accessor: 'name' },
        { id: 'nullValue', accessor: 'nullValue' },
        { id: 'undefinedValue', accessor: 'undefinedValue' },
      ],
    };

    const accessorFn = normalizeColumnAccessor(column);
    const result = accessorFn({
      name: 'test',
      nullValue: null,
      undefinedValue: undefined,
    });

    expect(result).toBe('test__JOIN____JOIN__');
  });
});
