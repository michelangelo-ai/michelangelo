import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { typeCellToString } from './type-cell-to-string';

import type { CellRenderer } from '#core/components/cell/types';
import type { TypeCellConfig } from './types';

export const TypeCell: CellRenderer<string, TypeCellConfig> = ({ value, column }) => {
  const content = typeCellToString({ value, column });

  if (!content) {
    return null;
  }

  return (
    <Tag
      size={TAG_SIZE.xSmall}
      behavior={TAG_BEHAVIOR.selection}
      hierarchy={TAG_HIERARCHY.secondary}
      color={TAG_COLOR.gray}
      closeable={false}
    >
      {content}
    </Tag>
  );
};

TypeCell.toString = typeCellToString;
