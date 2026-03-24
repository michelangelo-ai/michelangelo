import React, { useCallback, useMemo } from 'react';
import { useStyletron } from 'baseui';
import {
  ALIGNMENT,
  StyledParagraph,
  Tile,
  TILE_GROUP_KIND,
  TILE_KIND,
  TileGroup,
} from 'baseui/tile';
import { LabelMedium } from 'baseui/typography';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { TILE_GROUP_OVERRIDES, TILE_OVERRIDES } from './styled-components';

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
  labelAddon,
  format,
  parse,
  options,
}) => {
  const [, theme] = useStyletron();
  const { input, meta } = useField<string | boolean>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });

  const selectedIndex = useMemo(
    () => options.findIndex((option) => option.value === input.value),
    [options, input]
  );

  const selectTileAtIndex = useCallback(
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
      labelAddon={labelAddon}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <TileGroup
        kind={TILE_GROUP_KIND.singleSelect}
        onClick={selectTileAtIndex}
        selected={selectedIndex}
        // eslint-disable-next-line @typescript-eslint/prefer-nullish-coalescing
        disabled={disabled || readOnly}
        overrides={TILE_GROUP_OVERRIDES}
      >
        {options.map((option) => {
          return (
            <Tile
              key={String(option.value)}
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
