export interface TablePaginationProps {
  gotoPage: (pageIndex: number) => void;
  pageCount: number;
  setPageSize: (pageSize: number) => void;
  state: {
    pageSize: number;
    pageIndex: number;
  };
  pageSizes: PageSizeOption[];
  /**
   * When fetchPlugin is provided, enables "load more" behavior on the last page.
   */
  fetchPlugin?: {
    fetchNextPage: () => void;
    isFetchNextPageInProgress: boolean;
  };
}

export interface PageSizeOption {
  id: number;
  label: string;
}

export interface TablePaginationConfig {
  /**
   * The default number of rows to be displayed per page in a paginated table.
   * During runtime, users can modify page size.
   *
   * @default Smallest value in `pageSizes`
   */
  initialPageSize?: number;

  /**
   * Available page sizes for the table, formatted to provide to a dropdown
   * so user can modify page size during runtime.
   *
   * @default [15, 25, 50]
   */
  pageSizes?: PageSizeOption[];
}
