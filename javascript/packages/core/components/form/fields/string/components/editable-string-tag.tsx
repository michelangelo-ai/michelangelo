import { useState } from 'react';
import { useStyletron } from 'baseui';
import { ADJOINED, SIZE, StyledInput } from 'baseui/input';

import { Icon } from '#core/components/icon/icon';
import { TAG_BEHAVIOR, TAG_HIERARCHY } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';

import type { KeyboardEvent } from 'react';
import type { EditableStringTagProps } from './types';

export function EditableStringTag(props: EditableStringTagProps) {
  const { closeable, index, onRemove, readOnly, updateValue, value: initialValue } = props;
  const [, theme] = useStyletron();

  const [editing, setEditing] = useState(false);
  const [localValue, setLocalValue] = useState(initialValue);

  const handleCancelEditing = () => {
    setLocalValue(initialValue);
    setEditing(false);
  };

  const persistEditedValue = () => {
    updateValue(localValue, index);
    setEditing(false);
  };

  const handleConfirmOnEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      persistEditedValue();
    }
  };

  if (editing && !readOnly) {
    return (
      <Tag
        contentMaxWidth="100%"
        overrides={{
          ActionIcon: {
            component: Icon,
            // Matches the size baseui's own small Tag action icon (e.g. the remove icon)
            // renders at, so the confirm icon doesn't look oversized next to it.
            props: { name: 'check', onMouseDown: persistEditedValue, size: theme.sizing.scale500 },
          },
          // Tighter padding than the shared Tag's default, to suit a compact tag-input list.
          // Spacing between tags is handled by the container's `gap` (see StringTagInput) rather
          // than margin here, since Tag's own overrides always win margin conflicts when merged.
          Root: {
            style: {
              paddingTop: theme.sizing.scale100,
              paddingRight: theme.sizing.scale200,
              paddingBottom: theme.sizing.scale100,
              paddingLeft: theme.sizing.scale200,
            },
          },
        }}
        behavior={TAG_BEHAVIOR.selection}
        hierarchy={TAG_HIERARCHY.secondary}
      >
        <StyledInput
          $adjoined={ADJOINED.none}
          autoFocus
          onBlur={handleCancelEditing}
          onChange={(e) => setLocalValue(e.target.value)}
          onKeyDown={handleConfirmOnEnter}
          size={localValue.length + 1}
          $size={SIZE.compact}
          style={{ padding: 0 }}
          value={localValue}
        />
      </Tag>
    );
  }

  return (
    <Tag
      closeable={closeable}
      onActionClick={onRemove}
      onClick={() => setEditing(true)}
      overrides={{
        // Tighter padding than the shared Tag's default, to suit a compact tag-input list.
        // Spacing between tags is handled by the container's `gap` (see StringTagInput) rather
        // than margin here, since Tag's own overrides always win margin conflicts when merged.
        Root: {
          style: {
            paddingTop: theme.sizing.scale100,
            paddingRight: theme.sizing.scale200,
            paddingBottom: theme.sizing.scale100,
            paddingLeft: theme.sizing.scale200,
          },
        },
      }}
      hierarchy={TAG_HIERARCHY.primary}
    >
      {initialValue}
    </Tag>
  );
}
