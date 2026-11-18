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

  it('should read protobuf Timestamp { seconds, nanos } objects', () => {
    const task = createTask({
      record: {
        startTime: { seconds: '1670007850', nanos: 1 },
        endTime: { seconds: 1670007910, nanos: 0 },
      },
    });

    expect(resolveTaskTiming(task)).toEqual({ start: 1670007850, end: 1670007910 });
  });
});
