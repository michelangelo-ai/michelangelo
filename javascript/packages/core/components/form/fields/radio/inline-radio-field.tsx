import React, { useCallback } from 'react';
import { Input } from 'baseui/input';
import { ALIGN, Radio as RadioItem, RadioGroup } from 'baseui/radio';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { Theme } from 'baseui';
import type { RadioFieldProps } from './types';

export const InlineRadioField: React.FC<RadioFieldProps> = ({
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
  const { input, meta } = useField<string | boolean>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
  });

  const isBoolean = options.every(({ value }) => typeof value === 'boolean');

  const handleChange = useCallback(
    (event: React.FormEvent<HTMLInputElement>) => {
      const strVal = event.currentTarget.value;
      input.onChange(isBoolean ? strVal === 'true' : strVal);
    },
    [input, isBoolean]
  );

  if (readOnly) {
    return (
      <FormControl
        label={label}
        required={required}
        description={description}
        caption={caption}
        error={meta.touched && meta.error ? meta.error : undefined}
      >
        <Input
          id={input.name}
          readOnly
          value={options.find((option) => option.value === input.value)?.label ?? ''}
          overrides={{
            InputContainer: {
              style: ({ $theme }: { $theme: Theme }) => ({
                backgroundColor: $theme.colors.backgroundPrimary,
              }),
            },
          }}
        />
      </FormControl>
    );
  }

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <RadioGroup
        name={input.name}
        value={typeof input.value === 'boolean' ? String(input.value) : input.value}
        align={align}
        disabled={disabled}
        onChange={handleChange}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        overrides={{
          RadioGroupRoot: {
            style: ({ $theme }: { $theme: Theme }) => ({ gap: $theme.sizing.scale600 }),
          },
        }}
      >
        {options.map((option) => (
          <RadioItem
            key={String(option.value)}
            value={typeof option.value === 'boolean' ? String(option.value) : option.value}
            disabled={option.disabled}
            overrides={{
              RadioMarkOuter: {
                style: ({ $theme }: { $theme: Theme }) => ({
                  height: $theme.sizing.scale600,
                  width: $theme.sizing.scale600,
                }),
              },
              RadioMarkInner: {
                style: ({ $checked, $theme }: { $checked: boolean; $theme: Theme }) => ({
                  height: $checked ? $theme.sizing.scale100 : $theme.sizing.scale400,
                  width: $checked ? $theme.sizing.scale100 : $theme.sizing.scale400,
                  transform: 'none',
                }),
              },
            }}
          >
            {option.label}
          </RadioItem>
        ))}
      </RadioGroup>
    </FormControl>
  );
};
