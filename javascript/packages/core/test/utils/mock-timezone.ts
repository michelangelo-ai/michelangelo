/**
 * Mocks Intl.DateTimeFormat to control timezone output in tests.
 * This ensures consistent timezone behavior across test files.
 *
 * @param defaultTimezone - The timezone to use when not explicitly set to UTC
 * @returns A function to restore the original Intl.DateTimeFormat
 *
 * @example
 * ```ts
 * const restore = mockTimezone('Europe/Amsterdam');
 *
 * afterAll(() => {
 *   restore();
 * });
 * ```
 */
export function mockTimezone(defaultTimezone = 'Europe/Amsterdam') {
  const originalDateTimeFormat = Intl.DateTimeFormat;

  global.Intl.DateTimeFormat = function (
    locale?: string | string[],
    options?: Intl.DateTimeFormatOptions
  ): Intl.DateTimeFormat {
    return new originalDateTimeFormat(locale, {
      ...options,
      timeZone: options?.timeZone === 'UTC' ? 'UTC' : defaultTimezone,
    });
  } as unknown as Intl.DateTimeFormatConstructor;

  return () => {
    global.Intl.DateTimeFormat = originalDateTimeFormat;
  };
}
