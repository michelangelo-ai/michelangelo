import React from 'react';
import { Checkbox, LABEL_PLACEMENT, STYLE_TYPE } from 'baseui/checkbox';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { Theme } from 'baseui';
import type { BooleanFieldProps } from './types';

export const BooleanField: React.FC<BooleanFieldProps> = ({
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
  checkboxLabel,
  toggle = false,
}) => {
  const { input, meta } = useField<boolean>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
  });

  const handleChange = (event: React.FormEvent<HTMLInputElement>) => {
    input.onChange(event.currentTarget.checked);
  };

  const displayLabel = checkboxLabel ?? (input.value ? 'Enabled' : 'Disabled');

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      <Checkbox
        checked={input.value ?? false}
        onChange={readOnly ? undefined : handleChange}
        onBlur={input.onBlur}
        disabled={disabled}
        checkmarkType={toggle ? STYLE_TYPE.toggle_round : STYLE_TYPE.default}
        labelPlacement={LABEL_PLACEMENT.right}
        overrides={{
          Label: {
            style: ({ $theme }: { $theme: Theme }) => ({
              fontSize: $theme.sizing.scale550,
            }),
          },
          Root: {
            style: readOnly ? { cursor: 'default' } : {},
          },
        }}
      >
        {displayLabel}
      </Checkbox>
    </FormControl>
  );
};
