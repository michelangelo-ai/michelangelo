import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';

import type { BaseFieldProps } from '#core/components/form/fields/types';

export const StringField: React.FC<BaseFieldProps<string>> = ({
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
}) => {
  const { input, meta } = useField<string>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
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
        value={input.value ?? ''}
        name={input.name}
        onChange={(e) => input.onChange(e.currentTarget.value)}
        onBlur={input.onBlur}
        onFocus={input.onFocus}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
      />
    </FormControl>
  );
};
