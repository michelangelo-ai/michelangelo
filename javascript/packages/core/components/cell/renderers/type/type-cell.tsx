import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { typeCellToString } from './type-cell-to-string';

import type { CellRenderer } from '#core/components/cell/types';
import type { TypeCellConfig } from './types';

/**
 * Cell renderer for type/kind values displayed as gray tags.
 *
 * Converts type enum values to human-readable format and displays in a small gray tag.
 * Returns null for empty values. Always uses gray color.
 *
 * @param props.value - Type value string (e.g., enum value)
 * @param props.column - Column configuration
 *
 * @example
 * ```tsx
 * // In table column definition
 * { id: 'type', label: 'Type', type: CellType.TYPE }
 * // value: "JOB_TYPE_TRAINING" → Gray tag "Training"
 * ```
 */
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
