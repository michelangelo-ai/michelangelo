import { useState } from 'react';
import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { StringTagInput } from './string-tag-input';

import type { Theme } from 'baseui';
import type { KeyboardEvent } from 'react';
import type { MultiStringFieldProps } from '../types';

export function MultiStringField({
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
}: MultiStringFieldProps) {
  const { input, meta } = useField<string[]>(name, {
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
  const valueList = Array.isArray(input.value) ? input.value : [];

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

  const handleTagEntry = (event: KeyboardEvent<HTMLInputElement>) => {
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

  const handlePersistValue = () => {
    input.onBlur();
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
              onBlur: handlePersistValue,
              onKeyDown: handleTagEntry,
              readOnly,
              removeValue: removeValueAtIndex,
              updateValue: updateValueAtIndex,
              valueList,
            },
            style: { width: 'auto', flexGrow: 1, padding: 0 },
          },
        }}
      />
    </FormControl>
  );
}
