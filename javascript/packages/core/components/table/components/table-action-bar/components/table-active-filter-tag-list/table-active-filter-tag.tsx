import { useState } from 'react';
import { useStyletron } from 'baseui';
import { PLACEMENT, StatefulPopover } from 'baseui/popover';
import { KIND, Tag } from 'baseui/tag';
import { Tooltip } from 'baseui/tooltip';

import { getColumnFilter } from '#core/components/table/components/filter/get-column-filter';
import { useFilterFactory } from '#core/components/table/components/filter/use-filter-factory';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';

import type { Theme } from 'baseui';
import type { ColumnConfig } from '#core/components/table/types/column-types';
import type { ActiveFilterTagProps } from './types';

export function ActiveFilterTag<TData = unknown>(props: ActiveFilterTagProps<TData>) {
  const { column, preFilteredRows } = props;
  const [css, theme] = useStyletron();
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [showTooltip, setShowTooltip] = useState(false);
  const [delayHandler, setDelayHandler] = useState<NodeJS.Timeout | null>(null);
  const createFilter = useFilterFactory();
  const filter = createFilter({
    id: column.id,
    type: column.type,
    label: column.label,
  } satisfies ColumnConfig<TData>);

  const showTooltipAfterDelay = () => {
    setDelayHandler(setTimeout(() => setShowTooltip(true), 1000));
  };

  const hideTooltipImmediately = () => {
    if (delayHandler) {
      clearTimeout(delayHandler);
    }
    setShowTooltip(false);
  };

  const FilterComponent = getColumnFilter(column.type);

  return (
    <StatefulPopover
      placement={PLACEMENT.bottomLeft}
      content={({ close }) => (
        <FilterComponent
          columnId={column.id}
          close={close}
          getFilterValue={column.getFilterValue}
          setFilterValue={column.setFilterValue}
          preFilteredRows={preFilteredRows}
        />
      )}
      onOpen={() => setIsMenuOpen(true)}
      onClose={() => setIsMenuOpen(false)}
    >
      <Tag
        kind={KIND.blue}
        overrides={
          isMenuOpen
            ? {
                Root: {
                  style: ({ $theme }: { $theme: Theme }) => ({
                    borderColor: $theme.colors.accent,
                    borderWidth: '2px',
                  }),
                },
              }
            : {}
        }
        onActionClick={() => column.setFilterValue(undefined)}
      >
        <Tooltip
          content={() => (
            <div className={css({ maxWidth: theme.sizing.scale4800, overflowWrap: 'break-word' })}>
              {filter.getActiveFilter(column.getFilterValue())}
            </div>
          )}
          placement={PLACEMENT.top}
          isOpen={showTooltip}
          onMouseEnter={showTooltipAfterDelay}
          onMouseLeave={hideTooltipImmediately}
          showArrow={true}
        >
          <TruncatedText>{filter.getFilterSummary(column.getFilterValue())}</TruncatedText>
        </Tooltip>
      </Tag>
    </StatefulPopover>
  );
}
