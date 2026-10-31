import type { Task } from '../types';

/**
 * Builds a unique scroll ID for a task based on its name.
 * Used to create DOM IDs for navigation targeting.
 */
export function buildTaskScrollId<TTaskRecord extends object>(task: Task<TTaskRecord>): string {
  return `task-${task.name}`;
}

/**
 * Scrolls to a specific task in the execution view with smooth animation.
 *
 * Subtask sections only exist in the DOM while their parent panel is expanded, so
 * callers may pass the ancestor chain to fall back to the closest task that is
 * actually rendered.
 *
 * @param task - The task to scroll to
 * @param ancestors - Ancestors of the task, ordered from root to direct parent
 */
export function handleScrollToTask<TTaskRecord extends object>(
  task: Task<TTaskRecord>,
  ancestors: Task<TTaskRecord>[] = []
): void {
  const element = findRenderedTask([task, ...[...ancestors].reverse()]);

  if (!element) {
    return;
  }

  const elementRect = element.getBoundingClientRect();

  window.scrollTo({
    top: window.scrollY + elementRect.top - 16,
    behavior: 'smooth',
  });
}

function findRenderedTask<TTaskRecord extends object>(
  candidates: Task<TTaskRecord>[]
): HTMLElement | null {
  for (const candidate of candidates) {
    const element = document.getElementById(buildTaskScrollId(candidate));
    if (element) return element;
  }
  return null;
}
