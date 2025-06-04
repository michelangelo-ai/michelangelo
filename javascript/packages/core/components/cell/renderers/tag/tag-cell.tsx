import { BEHAVIOR, COLOR, HIERARCHY, SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';

import type { CellRenderer } from '#core/components/cell/types';
import type { TagCellConfig } from './types';

export const TagCell: CellRenderer<string, TagCellConfig> = ({ value, column }) => {
  if (!value) {
    return null;
  }

  return (
    <Tag
      size={SIZE.xSmall}
      hierarchy={HIERARCHY.secondary}
      behavior={BEHAVIOR.selection}
      closeable={false}
      color={column.color ?? COLOR.gray}
      overrides={{
        Root: {
          style: {
            width: '120px',
            justifyContent: 'center',
          },
        },
      }}
    >
      {value}
    </Tag>
  );
};
