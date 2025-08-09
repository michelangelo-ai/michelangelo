import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { buildTablePaginationPropsFactory } from '../__fixtures__/table-pagination-factory';
import { TablePagination } from '../table-pagination';

describe('TablePagination', () => {
  const mockSetPageSize = vi.fn();
  const mockGoToPage = vi.fn();
  const mockFetchNextPage = vi.fn();

  const buildPaginationProps = buildTablePaginationPropsFactory({
    gotoPage: mockGoToPage,
    setPageSize: mockSetPageSize,
  });

  afterEach(() => {
    mockSetPageSize.mockClear();
    mockGoToPage.mockClear();
    mockFetchNextPage.mockClear();
  });

  test('calls gotoPage on page change', async () => {
    const user = userEvent.setup();

    render(
      <TablePagination {...buildPaginationProps()} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: /next/ }));
    expect(mockGoToPage).toHaveBeenCalledWith(1);
  });

  test('calls fetchNextPage when on the last page', async () => {
    const user = userEvent.setup();

    render(
      <TablePagination
        {...buildPaginationProps({
          state: { pageIndex: 3, pageSize: 10 },
          fetchPlugin: {
            fetchNextPage: mockFetchNextPage,
            isFetchNextPageInProgress: false,
          },
        })}
      />,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: /next/ }));
    await waitFor(() => expect(mockFetchNextPage).toHaveBeenCalled());
  });

  test('renders loading button when fetch is in progress', () => {
    render(
      <TablePagination
        {...buildPaginationProps({
          fetchPlugin: {
            fetchNextPage: mockFetchNextPage,
            isFetchNextPageInProgress: true,
          },
        })}
      />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { name: /loading Next/ })).toBeInTheDocument();
  });

  test('ensures current page does not exceed available pages', () => {
    render(
      <TablePagination
        {...buildPaginationProps({
          pageCount: 5,
          state: { pageIndex: 10, pageSize: 10 },
        })}
      />,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(
      screen.getByRole('button', { name: 'previous page. current page 5 of 5' })
    ).toBeInTheDocument();
  });

  test('renders page size selector with correct options', async () => {
    const user = userEvent.setup();
    render(
      <TablePagination {...buildPaginationProps()} />,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByText(/10/));
    const pageSizes = ['10', '20', '50'];
    for (const pageSize of pageSizes) {
      expect(screen.getByRole('option', { name: pageSize })).toBeInTheDocument();
    }
  });
});
