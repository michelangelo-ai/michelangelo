import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { TASK_STATE } from '../../../../constants';
import { buildTicks, buildTimetable } from '../build-timetable';

describe('buildTimetable', () => {
  it('should position tasks relative to the earliest start', () => {
    const first = createTask({ name: 'Fetch', record: { startTime: '1000', endTime: '1030' } });
    const second = createTask({ name: 'Train', record: { startTime: '1030', endTime: '1090' } });

    const { rows, totalSeconds, hasTiming } = buildTimetable([first, second]);

    expect(hasTiming).toBe(true);
    expect(totalSeconds).toBe(90);
    expect(
      rows.map(({ startOffset, durationSeconds }) => ({ startOffset, durationSeconds }))
    ).toEqual([
      { startOffset: 0, durationSeconds: 30 },
      { startOffset: 30, durationSeconds: 60 },
    ]);
  });

  it('should extend a running task without an end time to the current time', () => {
    const running = createTask({ state: TASK_STATE.RUNNING, record: { startTime: '1000' } });

    const [row] = buildTimetable([running], 1060).rows;

    expect(row.ongoing).toBe(true);
    expect(row.durationSeconds).toBe(60);
  });

  it('should treat a task known by one timestamp as a point in time', () => {
    const stage = createTask({ record: { endTime: '1000' } });

    const [row] = buildTimetable([stage]).rows;

    expect(row).toMatchObject({ startOffset: 0, durationSeconds: 0, ongoing: false });
  });

  it('should include subtasks as their own rows', () => {
    const child = createTask({ name: 'Compile', record: { startTime: '1010', endTime: '1020' } });
    const parent = createTask({
      name: 'Build',
      subTasks: [child],
      record: { startTime: '1000', endTime: '1030' },
    });

    const { rows } = buildTimetable([parent]);

    expect(rows.map((row) => [row.task.name, row.depth])).toEqual([
      ['Build', 0],
      ['Compile', 1],
    ]);
    expect(rows[1].ancestors).toEqual([parent]);
  });

  it('should report missing timing instead of collapsing every task to zero', () => {
    const { rows, hasTiming, ticks } = buildTimetable([createTask(), createTask()]);

    expect(hasTiming).toBe(false);
    expect(ticks).toEqual([]);
    expect(rows.every((row) => row.startOffset === undefined)).toBe(true);
  });
});

describe('buildTicks', () => {
  it('should space ticks at a readable interval across the span', () => {
    expect(buildTicks(90)).toEqual([
      { offsetSeconds: 0, label: '0s' },
      { offsetSeconds: 30, label: '30s' },
      { offsetSeconds: 60, label: '1m' },
      { offsetSeconds: 90, label: '1m 30s' },
    ]);
  });

  it('should return a single tick for an empty span', () => {
    expect(buildTicks(0)).toEqual([{ offsetSeconds: 0, label: '0s' }]);
  });
});
