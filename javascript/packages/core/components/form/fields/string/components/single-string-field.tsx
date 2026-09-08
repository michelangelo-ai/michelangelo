import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { SingleStringFieldProps } from '../types';

export function SingleStringField({
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
  labelEndEnhancer,
  format,
  parse,
  maxLength,
}: SingleStringFieldProps) {
  const { input, meta } = useField<string>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });
  const currentLength = input.value?.length ?? 0;

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelEndEnhancer={labelEndEnhancer}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
      counter={maxLength ? { length: currentLength, maxLength } : undefined}
    >
      <Input
        id={input.name}
        value={input.value ?? ''}
        name={input.name}
        onChange={(e) => input.onChange(e.currentTarget.value)}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
        maxLength={maxLength}
      />
    </FormControl>
  );
}
