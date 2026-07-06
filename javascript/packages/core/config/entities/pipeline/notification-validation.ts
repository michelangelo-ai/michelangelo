const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const INVALID_EMAIL_MESSAGE = 'Enter valid email addresses, e.g. user@example.com.';

/** Validation error for a list of recipient emails, or `undefined` if all are valid. */
export function getEmailValidationError(emails: string[]): string | undefined {
  return emails.some((email) => !EMAIL_REGEX.test(email.trim()))
    ? INVALID_EMAIL_MESSAGE
    : undefined;
}
