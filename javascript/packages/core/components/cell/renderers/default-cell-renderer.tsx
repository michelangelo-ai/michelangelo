import { useStyletron } from 'baseui';

import { cellTooltipHOC } from '#core/components/cell/components/tooltip/cell-tooltip-hoc';
import { getCellRenderer } from '#core/components/cell/get-cell-renderer';
import { useCellStyles } from '#core/components/cell/hooks';

import type { Cell, CellRendererProps } from '#core/components/cell/types';

export function DefaultCellRenderer(props: CellRendererProps<unknown, Cell>) {
  const [css] = useStyletron();
  const { column, record } = props;
  const style = useCellStyles({ record, style: column.style });

  const ColumnRendererComponent = getCellRenderer(props);
  const Component = column.tooltip
    ? cellTooltipHOC(ColumnRendererComponent)
    : ColumnRendererComponent;

  return (
    <div className={css(style)}>
      <Component {...props} CellComponent={DefaultCellRenderer} />
    </div>
  );
}
