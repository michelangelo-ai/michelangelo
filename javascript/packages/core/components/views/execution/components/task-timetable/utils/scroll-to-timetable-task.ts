import { buildTaskScrollId } from '#core/components/views/execution/utils/scroll-to-task';

import type { Task } from '#core/components/views/execution/types';

/**
 * Scrolls to the details section for a timetable row.
 *
 * Subtask details only exist in the DOM while their parent accordion is
 * expanded. When the clicked task is not rendered, walk from the nearest
 * parent toward the root (ancestors are stored root → parent on each row)
 * and land on the closest section that is.
 */
export function scrollToTimetableTask<TTaskRecord extends object>(
  task: Task<TTaskRecord>,
  ancestors: Task<TTaskRecord>[]
): void {
  const element = findScrollTarget(task, ancestors);
  if (!element) return;

  window.scrollTo({
    top: window.scrollY + element.getBoundingClientRect().top - 16,
    behavior: 'smooth',
  });
}

function findScrollTarget<TTaskRecord extends object>(
  task: Task<TTaskRecord>,
  ancestors: Task<TTaskRecord>[]
): HTMLElement | null {
  const own = document.getElementById(buildTaskScrollId(task));
  if (own) return own;

  for (let index = ancestors.length - 1; index >= 0; index -= 1) {
    const ancestor = document.getElementById(buildTaskScrollId(ancestors[index]));
    if (ancestor) return ancestor;
  }

  return null;
}
