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
