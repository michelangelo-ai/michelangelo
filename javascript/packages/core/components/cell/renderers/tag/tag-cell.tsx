import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';

import type { CellRenderer } from '#core/components/cell/types';
import type { TagCellConfig } from './types';

export const TagCell: CellRenderer<string, TagCellConfig> = ({ value, column }) => {
  if (!value) {
    return null;
  }

  return (
    <Tag
      size={TAG_SIZE.xSmall}
      hierarchy={TAG_HIERARCHY.secondary}
      behavior={TAG_BEHAVIOR.selection}
      closeable={false}
      color={column.color ?? TAG_COLOR.gray}
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
