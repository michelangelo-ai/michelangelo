import { Textarea } from 'baseui/textarea';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { TextareaFieldProps } from './types';

export const TextareaField: React.FC<TextareaFieldProps> = ({
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
  labelAddon,
  format,
  parse,
  rows,
  maxLength,
}) => {
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
      labelAddon={labelAddon}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
      counter={maxLength ? { length: currentLength, maxLength } : undefined}
    >
      <Textarea
        id={input.name}
        name={input.name}
        value={input.value ?? ''}
        onChange={(e) => input.onChange(e.currentTarget.value)}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
        rows={rows}
        maxLength={maxLength}
        overrides={{
          Input: {
            style: {
              resize: 'vertical',
            },
          },
        }}
      />
    </FormControl>
  );
};
