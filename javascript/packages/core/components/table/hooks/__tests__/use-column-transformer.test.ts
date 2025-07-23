import { renderHook } from '@testing-library/react';

import { useColumnTransformer } from '../use-column-transformer';

import type { TableColumn } from '../../types/column-types';

describe('useColumnTransformer', () => {
  it('should preserve original column properties', () => {
    const columns: TableColumn[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
        type: 'text',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    expect(transformedColumns[0].id).toBe('name');
    expect(transformedColumns[0].type).toBe('text');
  });

  it('should transform columns with label to header mapping', () => {
    const columns: TableColumn[] = [
      {
        id: 'name',
        label: 'Full Name',
        accessor: 'name',
      },
      {
        id: 'age',
        label: 'Age',
        accessor: 'age',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    expect(transformedColumns).toHaveLength(2);
    expect(transformedColumns[0].header).toBe('Full Name');
    expect(transformedColumns[1].header).toBe('Age');
  });

  it('should create working accessor functions', () => {
    const columns: TableColumn[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'user.name',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    const accessor = transformedColumns[0].accessor;
    const accessorFn = transformedColumns[0].accessorFn;
    const testData = { user: { name: 'John Doe' } };

    expect(accessor!(testData)).toBe('John Doe');
    expect(accessorFn!(testData)).toBe('John Doe');
  });

  it('should memoize results when columns reference does not change', () => {
    const columns: TableColumn[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
      },
    ];

    const { result, rerender } = renderHook(() => useColumnTransformer(columns));
    const firstResult = result.current;

    rerender();
    const secondResult = result.current;

    expect(firstResult).toBe(secondResult);
  });

  it('should update results when columns change', () => {
    const initialColumns: TableColumn[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
      },
    ];

    const updatedColumns: TableColumn[] = [
      {
        id: 'name',
        label: 'Full Name',
        accessor: 'name',
      },
    ];

    const { result, rerender } = renderHook(({ columns }) => useColumnTransformer(columns), {
      initialProps: { columns: initialColumns },
    });

    const firstResult = result.current;
    expect(firstResult[0].header).toBe('Name');

    rerender({ columns: updatedColumns });
    const secondResult = result.current;
    expect(secondResult[0].header).toBe('Full Name');
    expect(firstResult).not.toBe(secondResult);
  });
});
