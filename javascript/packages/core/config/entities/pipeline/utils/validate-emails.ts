import type { FieldValidator } from '#core/components/form/validation/types';

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const validateEmails: FieldValidator = (value) => {
  const emails = Array.isArray(value) ? value : [];
  const allValid = emails.every((email) => typeof email === 'string' && EMAIL_REGEX.test(email));
  return allValid ? undefined : 'Must be a valid email.';
};
