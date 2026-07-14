import { forwardRef } from 'react';
import { useStyletron } from 'baseui';
import { DeleteAlt } from 'baseui/icon';
import { ADJOINED, SIZE, StyledInput } from 'baseui/input';
import { StyledClearIcon, StyledIconsContainer, StyledValueContainer } from 'baseui/select';

import { EditableStringTag } from './editable-string-tag';

import type { MouseEvent } from 'react';
import type { StringTagInputProps } from './types';

export const StringTagInput = forwardRef<HTMLInputElement, StringTagInputProps>(
  function StringTagInput(props, ref) {
    const { clear, readOnly, removeValue, updateValue, value, valueList, ...restProps } = props;
    const [, theme] = useStyletron();

    const handleClearInput = () => {
      clear();

      if (ref && typeof ref !== 'function') {
        ref.current?.focus();
      }
    };

    // Clicking anywhere in the tag list's empty space should focus the text input, the same way
    // it would for a native text field — but not when the click lands on a tag (e.g. to edit it).
    const handleContainerClick = (event: MouseEvent<HTMLDivElement>) => {
      if (event.target === event.currentTarget && ref && typeof ref !== 'function') {
        ref.current?.focus();
      }
    };

    return (
      <>
        <StyledValueContainer
          $multi={true}
          $style={{ gap: theme.sizing.scale100 }}
          onClick={handleContainerClick}
        >
          {valueList.map((tagValue, index) => (
            <EditableStringTag
              key={index}
              value={tagValue}
              index={index}
              closeable={!readOnly}
              onRemove={() => removeValue(index)}
              readOnly={readOnly}
              updateValue={updateValue}
            />
          ))}
          <StyledInput
            {...restProps}
            $adjoined={ADJOINED.none}
            readOnly={readOnly}
            ref={ref}
            // Sized to the typed content (not the browser's ~20-character default) so the input
            // takes minimal space on its current line instead of forcing itself onto a new one
            // whenever the remaining space on the current line is narrower than that default.
            size={String(value ?? '').length + 1}
            $size={SIZE.compact}
            value={value}
          />
        </StyledValueContainer>
        {valueList.length > 0 && !readOnly && (
          <StyledIconsContainer>
            <StyledClearIcon onClick={handleClearInput}>
              <DeleteAlt />
            </StyledClearIcon>
          </StyledIconsContainer>
        )}
      </>
    );
  }
);
