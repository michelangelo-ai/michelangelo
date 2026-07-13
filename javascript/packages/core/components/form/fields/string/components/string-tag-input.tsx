import { forwardRef } from 'react';
import { useStyletron } from 'baseui';
import { DeleteAlt } from 'baseui/icon';
import { ADJOINED, SIZE, StyledInput } from 'baseui/input';
import { StyledClearIcon, StyledIconsContainer, StyledValueContainer } from 'baseui/select';

import { StringTag } from './string-tag';

import type { FocusEvent } from 'react';
import type { StringTagInputProps } from './types';

export const StringTagInput = forwardRef<HTMLInputElement, StringTagInputProps>(
  function StringTagInput(props, ref) {
    const {
      clear,
      onBlur: propsOnBlur,
      persistOnBlur,
      readOnly,
      removeValue,
      updateValue,
      valueList,
      ...restProps
    } = props;
    const [, theme] = useStyletron();

    const handleClearInput = () => {
      clear();

      if (ref && typeof ref !== 'function') {
        ref.current?.focus();
      }
    };

    const handlePersistOnBlur = (event: FocusEvent<HTMLInputElement>) => {
      propsOnBlur?.(event);
      persistOnBlur();
    };

    return (
      <>
        <StyledValueContainer $multi={true} $style={{ gap: theme.sizing.scale100 }}>
          {valueList.map((value, index) => (
            <StringTag
              key={index}
              value={value}
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
            onBlur={handlePersistOnBlur}
            readOnly={readOnly}
            ref={ref}
            $size={SIZE.compact}
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
