import { useState } from 'react';
import { ADJOINED, SIZE, StyledInput } from 'baseui/input';

import { Icon } from '#core/components/icon/icon';
import { TAG_BEHAVIOR, TAG_HIERARCHY } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';

import type { KeyboardEvent } from 'react';
import type { StringTagProps } from './types';

// Tighter sizing than the shared Tag's default, to suit a compact tag-input list.
// Longhand properties (not `margin`/`padding` shorthand) to match getTagOverrides' own longhand
// Root override — mixing shorthand and longhand for the same property in one style object is
// unsupported by styletron's atomic rendering and resolves by CSS class insertion order, not
// object key order.
const TAG_ROOT_STYLE = {
  marginTop: '2px',
  marginRight: '2px',
  marginBottom: '2px',
  marginLeft: '2px',
  paddingTop: '4px',
  paddingRight: '6px',
  paddingBottom: '4px',
  paddingLeft: '6px',
};

export function StringTag(props: StringTagProps) {
  const { closeable, index, onRemove, readOnly, updateValue, value: initialValue } = props;

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
            props: { name: 'check', onMouseDown: persistEditedValue },
          },
          Root: { style: TAG_ROOT_STYLE },
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
      overrides={{ Root: { style: TAG_ROOT_STYLE } }}
      hierarchy={TAG_HIERARCHY.primary}
    >
      {initialValue}
    </Tag>
  );
}
