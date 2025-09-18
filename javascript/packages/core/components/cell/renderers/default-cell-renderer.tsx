import { useStyletron } from 'baseui';

import { CellEnhancer } from '#core/components/cell/components/cell-enhancer';
import { cellTooltipHOC } from '#core/components/cell/components/tooltip/cell-tooltip-hoc';
import { useCellStyles } from '#core/components/cell/hooks';
import { CellContainer } from '#core/components/cell/styled-components';
import { useGetCellRenderer } from '#core/components/cell/use-get-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';

import type { Cell, CellRendererProps } from '#core/components/cell/types';

export function DefaultCellRenderer(props: CellRendererProps<unknown, Cell>) {
  const [css] = useStyletron();
  const { column, record } = props;
  const resolver = useInterpolationResolver();
  const resolvedColumn = resolver(column, { row: record });
  const style = useCellStyles({ record, style: resolvedColumn.style });

  const getCellRenderer = useGetCellRenderer();
  const ColumnRendererComponent = getCellRenderer(props);
  const Component = resolvedColumn.tooltip
    ? cellTooltipHOC(ColumnRendererComponent)
    : ColumnRendererComponent;

  return (
    <CellContainer>
      <div className={css(style)}>
        <Component {...props} column={resolvedColumn} CellComponent={DefaultCellRenderer} />
      </div>
      <CellEnhancer endEnhancer={resolvedColumn.endEnhancer} />
    </CellContainer>
  );
}
