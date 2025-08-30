import { useStyletron } from 'baseui';

import { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
import { Icon } from '#core/components/icon/icon';
import { getObjectValue } from '#core/utils/object-utils';

import type { CellRenderer } from '#core/components/cell/types';
import type { MultiCellConfig } from './types';

export const MultiCell: CellRenderer<unknown, MultiCellConfig> = (props) => {
  const [css, theme] = useStyletron();
  const { column, record, CellComponent = DefaultCellRenderer } = props;
  const { items } = column;

  if (!columnHasData(column, record)) {
    return <>{`\u2014`}</>;
  }

  return (
    <div className={css(COLUMN_WRAPPER)}>
      {column.icon && <Icon name={column.icon} size={theme.sizing.scale600} width="32px" />}
      <div className={css(ITEMS_WRAPPER)}>
        {items.map((item, index) => {
          const value = getObjectValue<unknown>(record, item.accessor ?? item.id);
          return <CellComponent key={index} {...props} column={item} value={value ?? ''} />;
        })}
      </div>
    </div>
  );
};

function columnHasData(column: MultiCellConfig, record: object): boolean {
  return column.items.some((item) => getObjectValue<unknown>(record, item.accessor ?? item.id));
}

const COLUMN_WRAPPER = { alignItems: 'center', display: 'flex', gap: '12px' };
const ITEMS_WRAPPER = { display: 'grid', gap: '4px' };
