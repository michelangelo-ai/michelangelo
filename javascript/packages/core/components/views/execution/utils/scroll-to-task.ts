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
 * Subtask details only exist in the DOM while their parent accordion is
 * expanded. Callers may pass an ancestor chain (root → parent) so the scroll
 * falls back to the nearest rendered section when the task itself is not mounted.
 *
 * @param task - The task to scroll to
 * @param ancestors - Optional ancestors ordered from root to direct parent
 */
export function handleScrollToTask<TTaskRecord extends object>(
  task: Task<TTaskRecord>,
  ancestors: Task<TTaskRecord>[] = []
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
