import { useStyletron } from 'baseui';
import { Button, KIND } from 'baseui/button';
import { Pagination as BasePagination } from 'baseui/pagination';

import { TablePageSizeSelector } from './table-page-size-selector';

import type { TablePaginationProps } from './types';

export function TablePagination(props: TablePaginationProps) {
  const { state, pageCount, gotoPage, fetchPlugin } = props;
  const { pageIndex } = state;
  const [css] = useStyletron();

  // Ensure current page doesn't exceed available pages
  const currentPage = Math.min(pageIndex + 1, pageCount);

  return (
    <div
      className={css({
        display: 'flex',
        justifyContent: 'space-between',
        width: '100%',
        alignItems: 'center',
      })}
    >
      <TablePageSizeSelector
        pageSizes={props.pageSizes}
        state={props.state}
        setPageSize={props.setPageSize}
      />
      <BasePagination
        numPages={pageCount}
        currentPage={currentPage}
        onPageChange={({ nextPage }) => {
          // Trigger server fetch when reaching last page in server-side mode
          if (fetchPlugin && nextPage === pageCount) {
            fetchPlugin.fetchNextPage();
          }
          gotoPage(Math.max(0, nextPage - 1));
        }}
        overrides={{
          Root: { style: { display: 'flex', justifyContent: 'flex-end' } },
          // Hide max label for server-side pagination (unknown total)
          MaxLabel: fetchPlugin ? { component: () => null } : {},
          NextButton: fetchPlugin?.isFetchNextPageInProgress ? { component: LoadingButton } : {},
        }}
      />
    </div>
  );
}

// BaseUI requires a stable component reference in overrides — an inline arrow function
// would remount the button on every render. Keeping it here avoids a one-off file.
// eslint-disable-next-line react/no-multi-comp
function LoadingButton() {
  return (
    <Button isLoading={true} kind={KIND.tertiary}>
      Next
    </Button>
  );
}
