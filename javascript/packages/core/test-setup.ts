import { vi } from 'vitest';
import '@testing-library/jest-dom';

/**
 * Global setup for userEvent + vitest timer compatibility
 * This allows userEvent.setup({ advanceTimers: vi.advanceTimersByTime.bind(vi) })
 * to work correctly in all test files
 *
 * {@link https://github.com/testing-library/user-event/issues/1115}
 */
vi.stubGlobal('jest', {
  advanceTimersByTime: vi.advanceTimersByTime.bind(vi) as (ms: number) => void,
});

/**
 * Force UTC timezone for all tests to prevent timezone-related test failures
 * This ensures consistent behavior across different development and CI environments
 */
process.env.TZ = 'UTC';

// Verify UTC is properly configured
const timezoneOffset = new Date().getTimezoneOffset();
if (timezoneOffset !== 0) {
  console.warn(`⚠️  Expected UTC timezone but got offset: ${timezoneOffset} minutes`);
}

/**
 * Mock ResizeObserver for components that use it (like sticky table columns)
 * We mock it to prevent real observation side effects in tests
 */
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
