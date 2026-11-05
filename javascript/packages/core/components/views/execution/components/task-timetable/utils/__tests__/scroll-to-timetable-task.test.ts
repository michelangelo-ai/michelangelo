import { vi } from 'vitest';

import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { scrollToTimetableTask } from '../scroll-to-timetable-task';

const mockScrollTo = vi.fn();
const mockGetElementById = vi.fn();
const mockGetBoundingClientRect = vi.fn();

Object.defineProperty(global, 'window', {
  value: {
    scrollTo: mockScrollTo,
    scrollY: 100,
  },
  writable: true,
});

Object.defineProperty(global, 'document', {
  value: {
    getElementById: mockGetElementById,
  },
  writable: true,
});

describe('scrollToTimetableTask', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('should scroll to the task when its section is rendered', () => {
    const task = createTask({ name: 'Child' });
    const mockElement = { getBoundingClientRect: mockGetBoundingClientRect };

    mockGetElementById.mockReturnValue(mockElement);
    mockGetBoundingClientRect.mockReturnValue({ top: 200 });

    scrollToTimetableTask(task, []);

    expect(mockGetElementById).toHaveBeenCalledWith('task-Child');
    expect(mockScrollTo).toHaveBeenCalledWith({
      top: 100 + 200 - 16,
      behavior: 'smooth',
    });
  });

  it('should fall back to the nearest rendered ancestor', () => {
    const grandparent = createTask({ name: 'Grandparent' });
    const parent = createTask({ name: 'Parent' });
    const task = createTask({ name: 'Child' });
    const mockElement = { getBoundingClientRect: mockGetBoundingClientRect };

    // ancestors are stored root → parent on timetable rows
    mockGetElementById.mockImplementation((id: string) =>
      id === 'task-Grandparent' ? mockElement : null
    );
    mockGetBoundingClientRect.mockReturnValue({ top: 200 });

    scrollToTimetableTask(task, [grandparent, parent]);

    expect(mockGetElementById.mock.calls.map(([id]: [string]) => id)).toEqual([
      'task-Child',
      'task-Parent',
      'task-Grandparent',
    ]);
    expect(mockScrollTo).toHaveBeenCalledWith({ top: 100 + 200 - 16, behavior: 'smooth' });
  });

  it('should no-op when nothing in the chain is rendered', () => {
    mockGetElementById.mockReturnValue(null);

    scrollToTimetableTask(createTask({ name: 'Missing' }), [createTask({ name: 'Parent' })]);

    expect(mockScrollTo).not.toHaveBeenCalled();
  });
});
