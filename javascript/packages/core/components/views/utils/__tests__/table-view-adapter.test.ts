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

  it('should handle minimal TableConfig with only columns', () => {
    const minimalConfig = {
      columns: [
        { id: 'name', label: 'Name' },
        { id: 'status', label: 'Status' },
      ],
    };
    const runtimeProps = {
      data: [{ name: 'Item 1', status: 'Active' }],
      loading: false,
      error: undefined as ApplicationError | undefined,
    };
    const result = adaptTableConfigToTableProps(minimalConfig, runtimeProps);

    expect(result).toEqual({
      data: runtimeProps.data,
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

  it('returns undefined actions when config has no actions', () => {
    const result = adaptTableConfigToTableProps(buildTableConfig(), { data: [{ name: 'Item 1', status: 'Active' }], loading: false, error: undefined });
    expect(result.actions).toBeUndefined();
  });

  it('returns a render function when actions are configured', () => {
    // Full actions interaction is tested at the PhaseEntityView level.
    const tableConfig = buildTableConfig({
      actions: [{ display: { label: 'Delete' }, component: () => null }],
    });
    const result = adaptTableConfigToTableProps(tableConfig, { data: [{ name: 'Item 1', status: 'Active' }], loading: false, error: undefined });
    expect(typeof result.actions).toBe('function');
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

      expect(adaptTableConfigToTableProps(tableConfig, { data: [{ name: 'Item 1', status: 'Active' }], loading: false, error: undefined }).actionBarConfig).toEqual(
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

    const result = adaptTableConfigToTableProps(config, { data: [{ name: 'Item 1', status: 'Active' }], loading: false, error: undefined });

    expect(result.actionBarConfig).toEqual({
      enableSearch: !config.disableSearch,
      enableFilters: !config.disableFilters,
    });

    const { actionBarConfig: _actionBarConfig, ...passedThroughProps } = result;

    const runtimeProps = { data: [{ name: 'Item 1', status: 'Active' }], loading: false, error: undefined };

    const {
      disableSearch: _disableSearch,
      disableFilters: _disableFilters,
      ...expectedProps
    } = { ...config, ...runtimeProps };

    expect(passedThroughProps).toEqual(expectedProps);
  });
});
