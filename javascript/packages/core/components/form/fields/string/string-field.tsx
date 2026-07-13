import { useState } from 'react';
import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { StringTagInput } from './components/string-tag-input';

import type { Theme } from 'baseui';
import type { KeyboardEvent } from 'react';
import type { StringFieldProps } from './types';

export function StringField({
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
  multi = false,
}: StringFieldProps) {
  const { input, meta } = useField<string | string[]>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
    format,
    parse,
  });

  const [unpersistedValue, setUnpersistedValue] = useState('');

  // react-final-form defaults an unset field's value to '', so an array check (rather than a
  // truthiness/cast check) is needed to treat that default as an empty tag list.
  const valueList = multi && Array.isArray(input.value) ? input.value : [];

  const persistValue = (value: string) => {
    input.onChange([...valueList, value]);
    setUnpersistedValue('');
  };

  const removeValueAtIndex = (index: number) =>
    input.onChange(valueList.filter((_, i) => i !== index));

  const updateValueAtIndex = (newValue: string, index: number) => {
    const newList = [...valueList];
    newList[index] = newValue;
    input.onChange(newList);
  };

  const clearValueList = () => {
    input.onChange([]);
    setUnpersistedValue('');
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (readOnly) return;

    const isPersistingValue = event.key === 'Enter' && unpersistedValue;
    const isRemovingPersistedValue =
      event.key === 'Backspace' && valueList.length > 0 && !unpersistedValue;

    if (isPersistingValue) {
      event.preventDefault();
      persistValue(unpersistedValue);
    } else if (isRemovingPersistedValue) {
      removeValueAtIndex(valueList.length - 1);
    }
  };

  const persistOnBlur = () => {
    if (unpersistedValue) {
      persistValue(unpersistedValue);
    }
  };

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      labelEndEnhancer={labelEndEnhancer}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      {multi ? (
        <Input
          {...input}
          id={name}
          value={unpersistedValue}
          onChange={(e) => setUnpersistedValue(e.currentTarget.value)}
          placeholder={!disabled && !readOnly && valueList.length === 0 ? placeholder : ''}
          readOnly={readOnly}
          disabled={disabled}
          overrides={{
            InputContainer: {
              style: ({ $theme }: { $theme: Theme }) =>
                readOnly && !disabled ? { backgroundColor: $theme.colors.backgroundPrimary } : {},
            },
            Input: {
              component: StringTagInput,
              props: {
                clear: clearValueList,
                onKeyDown,
                readOnly,
                removeValue: removeValueAtIndex,
                updateValue: updateValueAtIndex,
                valueList,
                persistOnBlur,
              },
              style: { width: 'auto', flexGrow: 1, padding: 0 },
            },
          }}
        />
      ) : (
        <Input
          id={input.name}
          // cast: field value is string | string[]; only a string when multi is false
          value={(input.value as string) ?? ''}
          name={input.name}
          onChange={(e) => input.onChange(e.currentTarget.value)}
          onBlur={input.onBlur}
          onFocus={input.onFocus}
          placeholder={placeholder}
          readOnly={readOnly}
          disabled={disabled}
        />
      )}
    </FormControl>
  );
}
