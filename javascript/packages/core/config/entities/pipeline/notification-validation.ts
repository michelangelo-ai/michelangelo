import type { FieldValidator } from '#core/components/form/validation/types';

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const INVALID_EMAIL_MESSAGE = 'Enter valid email addresses, e.g. user@example.com.';

/**
 * Field-level validator for a list of recipient emails, for use with `MultiInputField`'s
 * `validate` prop — every entry must match a basic email shape.
 */
export const validateEmails: FieldValidator = (value) => {
  const emails = Array.isArray(value) ? (value as string[]) : [];
  return emails.some((email) => !EMAIL_REGEX.test(email.trim()))
    ? INVALID_EMAIL_MESSAGE
    : undefined;
};
