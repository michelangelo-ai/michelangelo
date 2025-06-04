import { useStyletron } from 'baseui';

import { getCellRenderer } from '#core/components/cell/get-cell-renderer';
import { useCellStyles } from '#core/components/cell/hooks';

import type { Cell, CellRendererProps } from '#core/components/cell/types';

export function DefaultCellRenderer(props: CellRendererProps<unknown, Cell>) {
  const [css] = useStyletron();
  const { column, record } = props;
  const style = useCellStyles({ record, style: column.style });

  const Component = getCellRenderer(props);

  return (
    <div className={css(style)}>
      <Component {...props} column={column} CellComponent={DefaultCellRenderer} />
    </div>
  );
}
