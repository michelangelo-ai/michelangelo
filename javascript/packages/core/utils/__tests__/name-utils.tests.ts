import { vi } from 'vitest';

import { generateSuffix, resolveTriggerRunTypePrefix } from '../name-utils';

const originalDate = Date;

beforeEach(() => {
  Object.defineProperty(global, 'crypto', {
    value: {
      randomUUID: vi.fn(() => 'abcd1234-5678-90ef-ghij-klmnopqrstuv'),
    },
    writable: true,
  });

  const mockDate = new Date('2024-01-01T12:00:00.000Z');
  // @ts-expect-error only mocking Date methods required for testing
  global.Date = vi.fn(() => mockDate);
  global.Date.parse = originalDate.parse;
});

afterEach(() => {
  global.Date = originalDate;
});

describe('generateSuffix', () => {
  it('should generate suffix without date by default', () => {
    const result = generateSuffix();
    expect(result).toBe('-abcd1234');
  });

  it('should generate suffix without date when withDate is false', () => {
    const result = generateSuffix({ withDate: false });
    expect(result).toBe('-abcd1234');
  });

  it('should generate suffix with date when withDate is true', () => {
    const result = generateSuffix({ withDate: true });
    expect(result).toBe('-20240101-120000-abcd1234');
  });
});

describe('resolveTriggerRunTypePrefix', () => {
  it('classifies a batch rerun regardless of the backfill flag', () => {
    expect(resolveTriggerRunTypePrefix('batchRerun', false)).toBe('batch-rerun');
    expect(resolveTriggerRunTypePrefix('batchRerun', true)).toBe('batch-rerun');
  });

  it('classifies a backfill window on a cron or interval trigger as a backfill', () => {
    expect(resolveTriggerRunTypePrefix('cronSchedule', true)).toBe('backfill');
    expect(resolveTriggerRunTypePrefix('intervalSchedule', true)).toBe('backfill');
  });

  it('classifies an interval trigger with no backfill window as interval', () => {
    expect(resolveTriggerRunTypePrefix('intervalSchedule', false)).toBe('interval');
  });

  it('falls back to cron for a cron trigger, or when the trigger type is unknown', () => {
    expect(resolveTriggerRunTypePrefix('cronSchedule', false)).toBe('cron');
    expect(resolveTriggerRunTypePrefix(undefined, false)).toBe('cron');
  });
});
