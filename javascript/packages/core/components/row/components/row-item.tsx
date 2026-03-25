import { useStyletron } from 'baseui';

import { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
import { getObjectValue } from '#core/utils/object-utils';
import { RowLabel } from './row-label';

import type { CellRenderer } from '#core/components/cell/types';
import type { RowProps } from '#core/components/row/types';

export const RowItem = (props: {
  item: RowProps['items'][number];
  record: NonNullable<RowProps['record']>;
  CellComponent?: CellRenderer<unknown>;
}) => {
  const [css, theme] = useStyletron();
  const { record, item, CellComponent = DefaultCellRenderer } = props;

  const value = getObjectValue(record, item.accessor ?? item.id);
  return (
    <div>
      <RowLabel label={item.label} />
      <div className={css(theme.typography.ParagraphSmall)}>
        <CellComponent value={value} column={item} record={record} />
      </div>
    </div>
  );
};
