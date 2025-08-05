import { useCallback, useState } from 'react';
import { Button, KIND, SHAPE, SIZE } from 'baseui/button';
import { PLACEMENT, StatefulPopover } from 'baseui/popover';

import { Icon } from '#core/components/icon/icon';
import { TableFilterMenuContent } from './components/table-filter-menu-content/table-filter-menu-content';

import type { FilterableColumn } from '#core/components/table/components/table-action-bar/types';
import type { TableData } from '#core/components/table/types/data-types';
import type { TableFilterMenuProps } from './types';

export function TableFilterMenu<T extends TableData = TableData>(props: TableFilterMenuProps<T>) {
  const { filterableColumns, columnFilters, setColumnFilters, preFilteredRows } = props;

  const [selectedColumn, setSelectedColumn] = useState<FilterableColumn<T> | undefined>();
  const [isMenuOpen, setIsMenuOpen] = useState<boolean>(false);

  const handleMenuClose = useCallback(() => {
    setSelectedColumn(undefined);
    setIsMenuOpen(false);
  }, []);

  return (
    <StatefulPopover
      placement={PLACEMENT.bottomLeft}
      onClose={handleMenuClose}
      onOpen={() => setIsMenuOpen(true)}
      overrides={{
        Body: {
          style: {
            // For some reason, the popover is not being positioned correctly when using
            // any combination of content prop that leverage filterableColumns.
            top: '44px',
          },
        },
      }}
      content={({ close }) => (
        <TableFilterMenuContent
          columnFilters={columnFilters}
          filterableColumns={filterableColumns}
          onClose={() => {
            handleMenuClose();
            close();
          }}
          preFilteredRows={preFilteredRows}
          selectedColumn={selectedColumn}
          setColumnFilters={setColumnFilters}
          setSelectedColumn={setSelectedColumn}
        />
      )}
    >
      <Button
        shape={SHAPE.pill}
        size={SIZE.compact}
        kind={isMenuOpen || columnFilters.length > 0 ? KIND.primary : KIND.secondary}
        startEnhancer={<Icon name="plus" size={16} />}
        overrides={{
          BaseButton: {
            style: {
              textWrap: 'nowrap',
            },
          },
        }}
      >
        Add filter
      </Button>
    </StatefulPopover>
  );
}
