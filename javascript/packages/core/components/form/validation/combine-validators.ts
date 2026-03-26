import type { FieldValidator } from './types';

export const combineValidators =
  (...validators: FieldValidator[]): FieldValidator =>
  (value) => {
    for (const validator of validators) {
      const error = validator(value);
      if (error !== undefined) return error;
    }
    return undefined;
  };
