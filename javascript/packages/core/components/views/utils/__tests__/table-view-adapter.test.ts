import { buildTableConfigFactory } from '#core/components/views/__fixtures__/table-config-factory';
import { adaptTableConfigToTableProps } from '../table-view-adapter';

import type { ApplicationError } from '#core/types/error-types';

describe('adaptTableConfigToTableProps', () => {
  const buildTableConfig = buildTableConfigFactory({
    columns: [
      { id: 'name', label: 'Name' },
      { id: 'status', label: 'Status' },
    ],
  });

  const mockRuntimeProps = {
    data: [{ name: 'Item 1', status: 'Active' }],
    loading: false,
    error: undefined,
  };

  it('should handle minimal TableConfig with only columns', () => {
    const minimalConfig = {
      columns: [
        { id: 'name', label: 'Name' },
        { id: 'status', label: 'Status' },
      ],
    };
    const result = adaptTableConfigToTableProps(minimalConfig, mockRuntimeProps);

    expect(result).toEqual({
      data: mockRuntimeProps.data,
      loading: false,
      error: undefined,
      columns: minimalConfig.columns,
      emptyState: undefined,
      actionBarConfig: {
        enableSearch: true,
        enableFilters: true,
      },
      disablePagination: undefined,
      disableSorting: undefined,
      pageSizes: undefined,
      enableStickySides: undefined,
    });
  });

  it('should handle loading state', () => {
    const tableConfig = buildTableConfig();
    const loadingRuntimeProps = {
      data: [],
      loading: true,
      error: undefined,
    };

    const result = adaptTableConfigToTableProps(tableConfig, loadingRuntimeProps);

    expect(result.loading).toBe(true);
    expect(result.data).toEqual([]);
  });

  it('should handle error state', () => {
    const tableConfig = buildTableConfig();
    const mockError: ApplicationError = {
      name: 'ApplicationError',
      message: 'Failed to load data',
      code: 500,
    };

    const errorRuntimeProps = {
      data: [],
      loading: false,
      error: mockError,
    };

    const result = adaptTableConfigToTableProps(tableConfig, errorRuntimeProps);

    expect(result.error).toBe(mockError);
    expect(result.loading).toBe(false);
  });

  describe('should correctly map disable flags to actionBar enables', () => {
    const testCases = [
      {
        description: 'both disabled',
        input: { disableSearch: true, disableFilters: true },
        expected: { enableSearch: false, enableFilters: false },
      },
      {
        description: 'both enabled',
        input: { disableSearch: false, disableFilters: false },
        expected: { enableSearch: true, enableFilters: true },
      },
      {
        description: 'mixed states',
        input: { disableSearch: true, disableFilters: false },
        expected: { enableSearch: false, enableFilters: true },
      },
      {
        description: 'undefined (defaults to enabled)',
        input: {},
        expected: { enableSearch: true, enableFilters: true },
      },
    ];

    test.each(testCases)('$description', ({ input, expected }) => {
      const tableConfig = buildTableConfig(input);

      expect(adaptTableConfigToTableProps(tableConfig, mockRuntimeProps).actionBarConfig).toEqual(
        expected
      );
    });
  });

  it('should pass through all TableConfig properties unchanged except actionBar transformation', () => {
    const config = buildTableConfig({
      columns: [{ id: 'test', label: 'Test' }],
      emptyState: { title: 'Empty', content: 'No data' },
      disablePagination: false,
      disableSorting: true,
      disableSearch: false,
      disableFilters: true,
      pageSizes: [{ id: 5, label: '5' }],
      enableStickySides: false,
    });

    const result = adaptTableConfigToTableProps(config, mockRuntimeProps);

    expect(result.actionBarConfig).toEqual({
      enableSearch: !config.disableSearch,
      enableFilters: !config.disableFilters,
    });

    const { actionBarConfig: _actionBarConfig, ...passedThroughProps } = result;

    const {
      disableSearch: _disableSearch,
      disableFilters: _disableFilters,
      ...expectedProps
    } = { ...config, ...mockRuntimeProps };

    expect(passedThroughProps).toEqual(expectedProps);
  });
});
