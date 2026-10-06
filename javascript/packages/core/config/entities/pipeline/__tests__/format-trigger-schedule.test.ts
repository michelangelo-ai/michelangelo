import { formatTriggerSchedule } from '#core/config/entities/pipeline/format-trigger-schedule';

describe('formatTriggerSchedule', () => {
  it('renders a cron expression', () => {
    expect(formatTriggerSchedule({ cronSchedule: { cron: '0 2 * * *' } })).toBe('cron 0 2 * * *');
  });

  it.each([
    ['86400s', 'every day'],
    ['3600s', 'every hour'],
    ['7200s', 'every 2 hours'],
    ['900s', 'every 15 minutes'],
    ['90s', 'every 90 seconds'],
  ])('renders an interval of %s as "%s"', (interval, expected) => {
    expect(formatTriggerSchedule({ intervalSchedule: { interval } })).toBe(expected);
  });

  it('names a batch rerun rather than describing a schedule', () => {
    expect(formatTriggerSchedule({ batchRerun: {} })).toBe('batch rerun');
  });

  it('returns an empty string for an undefined trigger', () => {
    expect(formatTriggerSchedule(undefined)).toBe('');
  });

  it('returns an empty string for a trigger with no schedule set', () => {
    expect(formatTriggerSchedule({})).toBe('');
  });

  it('returns an empty string for a cron trigger with an empty expression', () => {
    expect(formatTriggerSchedule({ cronSchedule: {} })).toBe('');
  });
});
