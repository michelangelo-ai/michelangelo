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
 * Uses modern scrollIntoView API with configurable navbar offset.
 *
 * @param task - The task to scroll to
 * @param options - Configuration for scroll behavior
 */
export function scrollToTask<TTaskRecord extends object>(task: Task<TTaskRecord>): void {
  const element = document.getElementById(buildTaskScrollId(task));

  if (!element) {
    return;
  }

  const elementRect = element.getBoundingClientRect();

  window.scrollTo({
    top: window.scrollY + elementRect.top - 16,
    behavior: 'smooth',
  });
}
