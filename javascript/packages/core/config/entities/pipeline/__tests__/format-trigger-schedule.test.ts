import { formatTriggerSchedule } from '#core/config/entities/pipeline/format-trigger-schedule';

describe('formatTriggerSchedule', () => {
  it('renders a cron expression', () => {
    expect(
      formatTriggerSchedule({ triggerType: { case: 'cronSchedule', value: { cron: '0 2 * * *' } } })
    ).toBe('cron 0 2 * * *');
  });

  it.each([
    [86400, 'every day'],
    [3600, 'every hour'],
    [7200, 'every 2 hours'],
    [900, 'every 15 minutes'],
    [90, 'every 90 seconds'],
  ])('renders an interval of %i seconds as "%s"', (seconds, expected) => {
    expect(
      formatTriggerSchedule({
        triggerType: { case: 'intervalSchedule', value: { interval: { seconds } } },
      })
    ).toBe(expected);
  });

  // Durations decode to protobuf-es Duration messages, whose `seconds` is a bigint.
  it('renders an interval given as a bigint', () => {
    expect(
      formatTriggerSchedule({
        triggerType: { case: 'intervalSchedule', value: { interval: { seconds: 3600n } } },
      })
    ).toBe('every hour');
  });

  it('names a batch rerun rather than describing a schedule', () => {
    expect(formatTriggerSchedule({ triggerType: { case: 'batchRerun', value: {} } })).toBe(
      'batch rerun'
    );
  });

  it('returns an empty string for an undefined trigger', () => {
    expect(formatTriggerSchedule(undefined)).toBe('');
  });

  it('returns an empty string for a trigger with no schedule set', () => {
    expect(formatTriggerSchedule({})).toBe('');
  });

  it('returns an empty string for a cron trigger with an empty expression', () => {
    expect(formatTriggerSchedule({ triggerType: { case: 'cronSchedule', value: {} } })).toBe('');
  });
});
