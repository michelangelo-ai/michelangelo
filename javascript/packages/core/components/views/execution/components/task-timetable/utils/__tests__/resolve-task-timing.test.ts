import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { resolveTaskTiming } from '../resolve-task-timing';

describe('resolveTaskTiming', () => {
  it('should read start and end times as epoch seconds', () => {
    const task = createTask({ record: { startTime: '100', endTime: '160' } });

    expect(resolveTaskTiming(task)).toEqual({ start: 100, end: 160 });
  });

  it('should read RFC 3339 timestamps', () => {
    const task = createTask({ record: { startTime: '2024-07-11T00:10:38Z' } });

    expect(resolveTaskTiming(task)).toEqual({ start: 1720656638, end: undefined });
  });

  it('should leave missing or unparseable bounds undefined', () => {
    const task = createTask({ record: { startTime: '', endTime: 'not a date' } });

    expect(resolveTaskTiming(task)).toEqual({ start: undefined, end: undefined });
  });
});
