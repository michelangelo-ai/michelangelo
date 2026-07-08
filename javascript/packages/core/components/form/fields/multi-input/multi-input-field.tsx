import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Input } from 'baseui/input';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { Tag } from '#core/components/tag/tag';

import type { KeyboardEvent } from 'react';
import type { MultiInputFieldProps } from './types';

// Stable reference: passing a fresh `[]` literal as `defaultValue` on every render would change
// identity each time, and react-final-form's `useField` re-registers the field whenever
// `defaultValue` changes identity — causing an infinite render loop.
const EMPTY_VALUE: string[] = [];

/**
 * Free-form multi-value text input: type a value and press Enter to add it as a removable
 * tag ("create-delete-tags" pattern).
 *
 * Use for lists of ad hoc strings (email addresses, Slack channel names) that aren't chosen
 * from a fixed set of options — unlike `SelectField`, this never presents an empty dropdown.
 */
export function MultiInputField({
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
}: MultiInputFieldProps) {
  const [css, theme] = useStyletron();
  const { input, meta } = useField<string[]>(name, {
    required,
    validate,
    defaultValue: defaultValue ?? EMPTY_VALUE,
    initialValue,
    label,
  });
  const [draft, setDraft] = useState('');

  const values = input.value ?? [];

  const commitDraft = () => {
    const nextValue = draft.trim();
    if (nextValue && !values.includes(nextValue)) {
      input.onChange([...values, nextValue]);
    }
    setDraft('');
    input.onBlur();
  };

  const removeValue = (removed: string) => {
    input.onChange(values.filter((value) => value !== removed));
  };

  const handleDraftKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      commitDraft();
    }
  };

  const handleDraftBlur = () => commitDraft();

  return (
    <>
      <FormControl
        label={label}
        required={required}
        description={description}
        labelEndEnhancer={labelEndEnhancer}
        caption={caption}
        error={meta.touched && meta.error ? meta.error : undefined}
      >
        <Input
          id={name}
          value={draft}
          onChange={(e) => setDraft(e.currentTarget.value)}
          onKeyDown={handleDraftKeyDown}
          onBlur={handleDraftBlur}
          onFocus={input.onFocus}
          placeholder={!disabled && !readOnly ? placeholder : ''}
          disabled={disabled}
          readOnly={readOnly}
        />
      </FormControl>
      {values.length > 0 && (
        <div
          className={css({
            display: 'flex',
            flexWrap: 'wrap',
            gap: theme.sizing.scale300,
            marginTop: theme.sizing.scale300,
          })}
        >
          {values.map((value) => (
            <Tag
              key={value}
              closeable={!disabled && !readOnly}
              onActionClick={() => removeValue(value)}
            >
              {value}
            </Tag>
          ))}
        </div>
      )}
    </>
  );
}
