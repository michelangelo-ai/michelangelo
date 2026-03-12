import React from 'react';
import { Checkbox, LABEL_PLACEMENT } from 'baseui/checkbox';

import { DescriptionText } from '#core/components/description-text';
import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { Theme } from 'baseui';
import type { CheckboxFieldProps } from './types';

export const CheckboxField: React.FC<CheckboxFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  description,
  caption,
  disabled,
  options,
}) => {
  const { input, meta } = useField<string[]>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
  });

  const selected = input.value ?? [];
  const hasDescriptions = options.some((option) => !!option.description);

  const toggle = (value: string) => {
    const next = selected.includes(value)
      ? selected.filter((v) => v !== value)
      : [...selected, value];
    input.onChange(next);
  };

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
      overrides={{
        ControlContainer: {
          style: ({ $theme }: { $theme: Theme }) => ({
            display: 'flex',
            flexDirection: hasDescriptions ? 'column' : 'row',
            flexWrap: hasDescriptions ? undefined : 'wrap',
            gap: $theme.sizing.scale600,
          }),
        },
      }}
    >
      <>
        {options.map((option) => (
          <Checkbox
            key={option.value}
            checked={selected.includes(option.value)}
            onChange={() => toggle(option.value)}
            onBlur={input.onBlur}
            disabled={disabled}
            labelPlacement={LABEL_PLACEMENT.right}
            overrides={{
              Label: {
                style: ({ $theme }: { $theme: Theme }) => ({ fontSize: $theme.sizing.scale550 }),
              },
            }}
          >
            {option.label}
            {option.description ? <DescriptionText>{option.description}</DescriptionText> : null}
          </Checkbox>
        ))}
      </>
    </FormControl>
  );
};
