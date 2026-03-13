import React from 'react';

import { FormControl } from '#core/components/form/components/form-control';
import { useField } from '#core/components/form/hooks/use-field';
import { Link } from '#core/components/link/link';
import { TruncatedText } from '#core/components/truncated-text/truncated-text';
import { isAbsoluteURL } from '#core/utils/string-utils';

import type { UrlFieldProps } from './types';

export const UrlField: React.FC<UrlFieldProps> = ({
  name,
  label,
  defaultValue,
  initialValue,
  required,
  validate,
  description,
  caption,
  placeholder,
  urlName,
}) => {
  const { input, meta } = useField<string>(name, {
    required,
    validate,
    defaultValue,
    initialValue,
    label,
  });

  const value = input.value;

  return (
    <FormControl
      label={label}
      required={required}
      description={description}
      caption={caption}
      error={meta.touched && meta.error ? meta.error : undefined}
    >
      {isAbsoluteURL(value) ? (
        <Link href={value}>
          <TruncatedText>{urlName ?? label ?? value}</TruncatedText>
        </Link>
      ) : (
        <span>{placeholder}</span>
      )}
    </FormControl>
  );
};
