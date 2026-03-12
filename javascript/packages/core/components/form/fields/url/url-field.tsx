import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { url } from '#core/components/form/validation/validators';

import type { BaseFieldProps } from '#core/components/form/fields/types';
import type { FieldValidator } from '#core/components/form/validation/types';

export const UrlField: React.FC<BaseFieldProps<string>> = ({
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
  const validators: FieldValidator[] = [url()];
  if (validate) validators.push(validate);

  const { input, meta } = useField<string>(name, {
    required,
    validate: combineValidators(...validators),
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
        value={input.value}
        name={input.name}
        onChange={(e) => input.onChange(e.currentTarget.value)}
        onBlur={input.onBlur}
        placeholder={placeholder}
        readOnly={readOnly}
        disabled={disabled}
      />
    </FormControl>
  );
};
