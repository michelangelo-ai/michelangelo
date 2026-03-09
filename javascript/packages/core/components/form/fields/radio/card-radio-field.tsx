import React, { useMemo, useCallback } from 'react';
import { useStyletron } from 'baseui';
import { ALIGN } from 'baseui/radio';
import { LabelMedium } from 'baseui/typography';
import {
  Tile,
  TileGroup,
  StyledParagraph,
  TILE_GROUP_KIND,
  TILE_KIND,
  ALIGNMENT,
} from 'baseui/tile';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import { getTileGroupOverrides, TILE_OVERRIDES } from './styled-components';

import type { RadioFieldProps } from './types';

export const CardRadioField: React.FC<RadioFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  disabled,
  description,
  caption,
  options,
  align = ALIGN.horizontal,
}) => {
  const [, theme] = useStyletron();
  const { input, meta } = useField<string | boolean>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
  });

  const selectedIndex = useMemo(
    () => options.findIndex((option) => option.value === input.value),
    [options, input]
  );

  const onClick = useCallback(
    (e: React.SyntheticEvent | KeyboardEvent, index: number) => {
      // Each tile is a button, so we need to prevent the default behavior and stop propagation to avoid form submission
      e.preventDefault();
      e.stopPropagation();
      input.onChange(options[index].value);
    },
    [input, options]
  );

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <TileGroup
        kind={TILE_GROUP_KIND.singleSelect}
        onClick={onClick}
        selected={selectedIndex}
        disabled={disabled || readOnly}
        overrides={getTileGroupOverrides(align)}
      >
        {options.map((option) => {
          return (
            <Tile
              tileKind={TILE_KIND.selection}
              leadingContent={() => (
                <LabelMedium $style={{ textAlign: 'left' }}>{option.label}</LabelMedium>
              )}
              headerAlignment={ALIGNMENT.left}
              bodyAlignment={ALIGNMENT.left}
              overrides={TILE_OVERRIDES}
            >
              <StyledParagraph $style={{ textAlign: 'left', marginBottom: theme.sizing.scale600 }}>
                {option.description}
              </StyledParagraph>
            </Tile>
          );
        })}
      </TileGroup>
    </FormControl>
  );
};
