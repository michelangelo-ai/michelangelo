import React from 'react';
import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { BaseFieldProps } from '#core/components/form/fields/types';

export const NumberField: React.FC<BaseFieldProps<number | undefined>> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  readOnly,
  disabled,
  placeholder,
  description,
  caption,
  format,
  parse,
}) => {
  const { input, meta } = useField<number | undefined>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });

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
        type="number"
        value={input.value == null ? '' : String(input.value)}
        name={input.name}
        onChange={(e) => input.onChange(parseNumber(e.currentTarget.value))}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
      />
    </FormControl>
  );
};

function parseNumber(s: string): number | undefined {
  if (s === '') return undefined;
  const n = Number(s);
  return isNaN(n) ? undefined : n;
}
