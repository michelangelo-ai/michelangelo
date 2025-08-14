import { vi } from 'vitest';

import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { buildTaskScrollId, scrollToTask } from '../scroll-to-task';

// Mock DOM and window methods
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

describe('scroll-to-task', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('buildTaskScrollId', () => {
    it('should create unique scroll ID from task name', () => {
      const task = createTask({ name: 'Build Pipeline' });

      expect(buildTaskScrollId(task)).toBe('task-Build Pipeline');
    });

    it('should handle task names with special characters', () => {
      const task = createTask({ name: 'Task with-spaces_and.dots' });

      expect(buildTaskScrollId(task)).toBe('task-Task with-spaces_and.dots');
    });
  });

  describe('scrollToTask', () => {
    it('should scroll to task with default offset', () => {
      const task = createTask({ name: 'Test Task' });
      const mockElement = { getBoundingClientRect: mockGetBoundingClientRect };

      mockGetElementById.mockReturnValue(mockElement);
      mockGetBoundingClientRect.mockReturnValue({ top: 200 });

      scrollToTask(task);

      expect(mockGetElementById).toHaveBeenCalledWith('task-Test Task');
      expect(mockScrollTo).toHaveBeenCalledWith({
        top: 100 + 200 - 16, // scrollY + elementTop - defaultOffset
        behavior: 'smooth',
      });
    });

    it('should handle missing element gracefully', () => {
      const task = createTask({ name: 'Missing Task' });

      mockGetElementById.mockReturnValue(null);

      scrollToTask(task);

      expect(mockGetElementById).toHaveBeenCalledWith('task-Missing Task');
      expect(mockScrollTo).not.toHaveBeenCalled();
    });
  });
});
