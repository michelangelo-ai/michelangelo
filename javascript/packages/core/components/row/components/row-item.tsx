import { useStyletron } from 'baseui';

import { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
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
  const { record, CellComponent = DefaultCellRenderer } = props;
  const resolver = useInterpolationResolver();
  const item = resolver(props.item, { row: record });

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
