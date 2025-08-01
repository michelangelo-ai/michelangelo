import type { Task, TaskState } from '#core/components/views/execution/types';

// Helper function to create test tasks
export function createTask(
  name: string,
  state: TaskState,
  focused = false,
  subTasks: Task[] = []
): Task {
  return {
    name,
    state,
    focused,
    subTasks,
    record: { name, state },
  };
}
