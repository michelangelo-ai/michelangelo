import { buildMatrix } from './utils/build-matrix';
import { buildTaskList } from './utils/build-task-list';

import type { ExecutionDetailViewSchema } from './types';

export function Execution<
  TData extends object = object,
  TTaskRecord extends object = object,
>(props: { schema: ExecutionDetailViewSchema<TData, TTaskRecord>; data: TData }) {
  const { schema, data } = props;
  const taskList = buildTaskList(schema, data);

  if (!taskList.length) {
    return (
      <div>
        <h3>{schema.emptyState.title}</h3>
        {schema.emptyState.description && <p>{schema.emptyState.description}</p>}
      </div>
    );
  }

  // TODO: Implement the styled execution view
  const matrix = buildMatrix(taskList);

  return (
    <div>
      <div>
        <h3>Overview</h3>
        <div>
          {matrix.map((item, index) => (
            <div key={index}>
              <h4>Task Matrix level {index}</h4>
              {item.taskList.map((task, index) => (
                <div key={index}>{task.name}</div>
              ))}
            </div>
          ))}
        </div>
      </div>

      <div>
        {taskList.map((task, index) => (
          <div key={index}>
            <h4>{task.name}</h4>
            <p>State: {task.state}</p>
            <p>Focused: {task.focused ? 'Yes' : 'No'}</p>
            {/* Task body content will be implemented in next iteration */}
          </div>
        ))}
      </div>
    </div>
  );
}
