import { createTask } from '#core/components/views/execution/components/task-details/__fixtures__/task-details-fixtures';
import { flattenTaskList } from '../flatten-task-list';

describe('flattenTaskList', () => {
  it('should walk the tree depth-first, tracking depth and ancestors', () => {
    const child = createTask({ name: 'Compile' });
    const parent = createTask({ name: 'Build', subTasks: [child] });
    const sibling = createTask({ name: 'Deploy' });

    expect(flattenTaskList([parent, sibling])).toEqual([
      { task: parent, depth: 0, ancestors: [] },
      { task: child, depth: 1, ancestors: [parent] },
      { task: sibling, depth: 0, ancestors: [] },
    ]);
  });
});
