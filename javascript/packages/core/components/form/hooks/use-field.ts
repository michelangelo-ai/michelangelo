import { useField as useReactFinalFormField } from 'react-final-form';

import { useFieldRegistration } from '#core/components/form/hooks/use-field-registration';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { required as requiredValidator } from '#core/components/form/validation/validators';

import type { FieldValidator } from '#core/components/form/validation/types';
import type { FieldInput, FieldState } from '../types';

export function useField<T = unknown>(
  name: string,
  options?: {
    validate?: FieldValidator;
    required?: boolean;
    defaultValue?: T;
    initialValue?: T;
    label?: string;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    parse?: (value: any) => T;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    format?: (value: T) => any;
  }
): { input: FieldInput<T>; meta: FieldState } {
  useFieldRegistration(name, options?.label);

  const composedValidate = options?.required
    ? combineValidators(requiredValidator(), ...(options.validate ? [options.validate] : []))
    : options?.validate;

  const validate = composedValidate ? (value: T) => composedValidate(value as unknown) : undefined;

  const field = useReactFinalFormField<T>(name, {
    validate,
    defaultValue: options?.defaultValue,
    initialValue: options?.initialValue,
    parse: options?.parse,
    format: options?.format,
  });

  const input: FieldInput<T> = {
    value: field.input.value,
    name: field.input.name,
    onChange: field.input.onChange,
    onBlur: field.input.onBlur,
    onFocus: field.input.onFocus,
  };

  const meta: FieldState = {
    error: typeof field.meta.error === 'string' ? field.meta.error : undefined,
    touched: !!field.meta.touched,
  };

  return { input, meta };
}
