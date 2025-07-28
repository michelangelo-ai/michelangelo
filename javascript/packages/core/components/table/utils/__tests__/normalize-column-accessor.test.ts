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
});
