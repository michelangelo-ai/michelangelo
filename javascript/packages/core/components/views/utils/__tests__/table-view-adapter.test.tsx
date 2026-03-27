import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildTableRowFactory } from '#core/components/table/__fixtures__/row-factory';
import { buildTableConfigFactory } from '#core/components/views/__fixtures__/table-config-factory';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { adaptTableConfigToTableProps } from '../table-view-adapter';

import type { ActionComponentProps } from '#core/components/actions/types';
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

  it('passes runtime data through to the result', () => {
    const result = adaptTableConfigToTableProps(buildTableConfig(), mockRuntimeProps);

    expect(result.data).toBe(mockRuntimeProps.data);
    expect(result.loading).toBe(false);
    expect(result.error).toBeUndefined();
  });

  it('passes columns from config', () => {
    const result = adaptTableConfigToTableProps(buildTableConfig(), mockRuntimeProps);

    expect(result.columns[0]).toMatchObject({ id: 'name', label: 'Name' });
    expect(result.columns[1]).toMatchObject({ id: 'status', label: 'Status' });
  });

  it('forwards loading state from runtime props', () => {
    const result = adaptTableConfigToTableProps(buildTableConfig(), {
      data: [],
      loading: true,
      error: undefined,
    });

    expect(result.loading).toBe(true);
    expect(result.data).toEqual([]);
  });

  it('forwards error from runtime props', () => {
    const mockError: ApplicationError = {
      name: 'ApplicationError',
      message: 'Failed to load data',
      code: 500,
    };

    const result = adaptTableConfigToTableProps(buildTableConfig(), {
      data: [],
      loading: false,
      error: mockError,
    });

    expect(result.error).toBe(mockError);
  });

  describe('actionBar config mapping', () => {
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

  it('forwards table display options from config', () => {
    const result = adaptTableConfigToTableProps(
      buildTableConfig({
        disablePagination: true,
        disableSorting: true,
        pageSizes: [{ id: 5, label: '5' }],
        enableStickySides: false,
        emptyState: { title: 'Empty', content: 'No data' },
      }),
      mockRuntimeProps
    );

    expect(result.disablePagination).toBe(true);
    expect(result.disableSorting).toBe(true);
    expect(result.pageSizes).toEqual(expect.arrayContaining([{ id: 5, label: '5' }]));
    expect(result.enableStickySides).toBe(false);
    expect(result.emptyState).toEqual({ title: 'Empty', content: 'No data' });
  });

  describe('actions wiring', () => {
    const buildRow = buildTableRowFactory<object>();

    it('returns undefined actions when config has no actions', () => {
      const result = adaptTableConfigToTableProps(buildTableConfig(), mockRuntimeProps);
      expect(result.actions).toBeUndefined();
    });

    it('renders an ActionsPopover when actions are configured', () => {
      const RunDialog = ({ isOpen }: ActionComponentProps) =>
        isOpen ? <div role="dialog">Run dialog</div> : null;
      const config = buildTableConfig({
        actions: [{ display: { label: 'Run' }, component: RunDialog }],
      });
      const result = adaptTableConfigToTableProps(config, mockRuntimeProps);

      const Actions = result.actions!;
      render(
        <Actions row={buildRow({ record: { id: '1' } })} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('button', { name: 'Actions' })).toBeInTheDocument();
    });

    it('shows action menu items when the trigger is clicked', async () => {
      const user = userEvent.setup();
      const DeleteDialog = ({ isOpen }: ActionComponentProps) =>
        isOpen ? <div role="dialog">Delete dialog</div> : null;
      const config = buildTableConfig({
        actions: [{ display: { label: 'Delete' }, component: DeleteDialog }],
      });
      const result = adaptTableConfigToTableProps(config, mockRuntimeProps);

      const Actions = result.actions!;
      render(
        <Actions row={buildRow({ record: { id: '1' } })} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Actions' }));
      expect(await screen.findByRole('option', { name: 'Delete' })).toBeInTheDocument();
    });

    it('passes row data to the action component', async () => {
      const user = userEvent.setup();
      const capturedData: Record<string, unknown>[] = [];
      const Component = ({ record, isOpen }: ActionComponentProps) => {
        if (isOpen) capturedData.push(record);
        return isOpen ? <div role="dialog" /> : null;
      };
      const config = buildTableConfig({
        actions: [{ display: { label: 'Run' }, component: Component }],
      });
      const result = adaptTableConfigToTableProps(config, mockRuntimeProps);

      const rowData = { id: '42', type: 'pipeline' };
      const Actions = result.actions!;
      render(
        <Actions row={buildRow({ record: rowData })} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Actions' }));
      await user.click(await screen.findByRole('option', { name: 'Run' }));
      expect(capturedData[0]).toEqual(rowData);
    });
  });
});
