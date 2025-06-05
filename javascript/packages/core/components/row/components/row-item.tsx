import { useStyletron } from 'baseui';

import { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
import { getObjectValue } from '#core/utils/object-utils';
import { RowLabel } from './row-label';

import type { RowProps } from '#core/components/row/types';

export const RowItem = (props: {
  item: RowProps['items'][number];
  record: NonNullable<RowProps['record']>;
}) => {
  const [css, theme] = useStyletron();
  const { record, item } = props;

  const value = getObjectValue(record, item.accessor ?? item.id);
  return (
    <div>
      <RowLabel label={item.label} />
      <div className={css(theme.typography.ParagraphSmall)}>
        <DefaultCellRenderer value={value} column={item} record={record} />
      </div>
    </div>
  );
};
