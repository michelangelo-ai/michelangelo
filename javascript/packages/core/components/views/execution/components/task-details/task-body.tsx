import { getObjectValue } from '#core/utils/object-utils';
import { TaskFlow } from '../task-flow';
import { TaskBodyStruct } from './renderers/task-body-struct';

import type { TaskBodyProps } from './types';

export function TaskBody<TTaskRecord extends object>(props: TaskBodyProps<TTaskRecord>) {
  const { task, bodySchema } = props;
  const { subTasks } = task;

  if (subTasks?.length) {
    return <TaskFlow taskList={subTasks} />;
  }

  if (bodySchema?.length) {
    return bodySchema.map((schema, index) => {
      const { label } = schema;
      const value = getObjectValue<unknown>(task.record, schema.accessor);

      if (schema.type === 'struct') {
        return <TaskBodyStruct key={index} label={label} value={value as object} />;
      }
    });
  }

  return <div>No subtasks, no body schema for {task.name}</div>;
}
