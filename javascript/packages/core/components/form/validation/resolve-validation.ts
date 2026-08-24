import { combineValidators } from '#core/components/form/validation/combine-validators';
import {
  max as maxValidator,
  maxLength as maxLengthValidator,
  min as minValidator,
  minLength as minLengthValidator,
  regex as regexValidator,
  url as urlValidator,
} from '#core/components/form/validation/validators';

import type { FieldValidation } from '#core/components/form/types/config-types';
import type { FieldValidator } from '#core/components/form/validation/types';

export function resolveValidation(
  validation: FieldValidation | undefined
): FieldValidator | undefined {
  if (!validation) return undefined;

  const validators: FieldValidator[] = [];

  if (validation.min !== undefined) validators.push(minValidator(validation.min));
  if (validation.max !== undefined) validators.push(maxValidator(validation.max));
  if (validation.minLength !== undefined) validators.push(minLengthValidator(validation.minLength));
  if (validation.maxLength !== undefined) validators.push(maxLengthValidator(validation.maxLength));

  if (validation.regex) {
    validators.push(regexValidator(validation.regex.pattern, validation.regex.errorMessage));
  }

  if (validation.url) {
    const errorMessage =
      typeof validation.url === 'object' ? validation.url.errorMessage : undefined;
    validators.push(urlValidator(errorMessage));
  }

  if (validation.validate) validators.push(validation.validate);

  if (validators.length === 0) return undefined;
  if (validators.length === 1) return validators[0];
  return combineValidators(...validators);
}
