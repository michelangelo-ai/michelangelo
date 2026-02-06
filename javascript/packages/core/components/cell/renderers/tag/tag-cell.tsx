import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';

import type { CellRenderer } from '#core/components/cell/types';
import type { TagCellConfig } from './types';

/**
 * Cell renderer for displaying values as small, centered tags.
 *
 * Renders text inside a fixed-width tag with customizable color. Useful for categories,
 * labels, or short status indicators. Returns null for empty values.
 *
 * @param props.value - Text to display in the tag
 * @param props.column - Column configuration with optional color
 *
 * @example
 * ```tsx
 * // In table column definition
 * { id: 'category', label: 'Type', type: CellType.TAG, color: TAG_COLOR.blue }
 * // Renders: [  Training  ] (centered tag, 120px width)
 * ```
 */
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
