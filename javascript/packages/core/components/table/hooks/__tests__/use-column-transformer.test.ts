import { CellContext } from '@tanstack/react-table';
import { renderHook, screen } from '@testing-library/react';
import { render } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { useColumnTransformer } from '../use-column-transformer';

import type { ColumnConfig } from '../../types/column-types';

describe('useColumnTransformer', () => {
  it('should pick id from column config', () => {
    const columns: ColumnConfig[] = [
      {
        id: 'name',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;
    expect(transformedColumns[0].id).toBe('name');
  });

  it('should preserve original column properties in meta', () => {
    const columns: ColumnConfig[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
        type: 'text',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    expect(transformedColumns[0].meta.id).toBe('name');
    expect(transformedColumns[0].meta.type).toBe('text');
    expect(transformedColumns[0].meta.label).toBe('Name');
    expect(transformedColumns[0].meta.accessor).toBe('name');
  });

  it('should transform columns with label to header mapping', () => {
    const columns: ColumnConfig[] = [
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

  it('should create working accessor function', () => {
    const columns: ColumnConfig[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'user.name',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    const accessorFn = transformedColumns[0].accessorFn;
    const testData = { user: { name: 'John Doe' } };

    expect(accessorFn(testData)).toBe('John Doe');
  });

  it('should memoize results when columns reference does not change', () => {
    const columns: ColumnConfig[] = [
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
    const initialColumns: ColumnConfig[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
      },
    ];

    const updatedColumns: ColumnConfig[] = [
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

  it('should handle columns without labels', () => {
    const columns: ColumnConfig[] = [
      {
        id: 'name',
        accessor: 'name',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumn = result.current[0];

    expect(transformedColumn.header).toBeUndefined();
  });

  it('should handle empty columns array', () => {
    const columns: ColumnConfig[] = [];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const transformedColumns = result.current;

    expect(transformedColumns).toHaveLength(0);
  });

  it('should render TableCell component when cell function is called', () => {
    const columns: ColumnConfig[] = [
      {
        id: 'name',
        label: 'Name',
        accessor: 'name',
      },
    ];

    const { result } = renderHook(() => useColumnTransformer(columns));
    const cellRenderer = result.current[0].cell;

    const mockCellContext = {
      column: {
        columnDef: {
          meta: { id: 'name', label: 'Name', accessor: 'name' },
        },
      },
      row: {
        original: { name: 'John Doe' },
      },
      getValue: () => 'John Doe',
    } as unknown as CellContext<unknown, unknown>;

    render(cellRenderer(mockCellContext), buildWrapper([getRouterWrapper()]));
    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });
});
