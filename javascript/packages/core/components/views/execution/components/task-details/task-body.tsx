import { useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { TaskContentStack } from '#core/components/views/execution/styled-components';
import { getObjectValue } from '#core/utils/object-utils';
import { TaskFlow } from '../task-flow';
import { TaskBodyMetadata } from './renderers/task-body-metadata';
import { TaskBodyStruct } from './renderers/task-body-struct';
import { TaskBodyTextarea } from './renderers/task-body-textarea';
import { TaskBodyMetadataSchema, TaskBodyTextareaSchema } from './renderers/types';
import { TaskDetails } from './task-details';

import type { TaskBodyProps } from './types';

export function TaskBody<TTaskRecord extends object>(props: TaskBodyProps<TTaskRecord>) {
  const [css, theme] = useStyletron();
  const { task, bodySchema } = props;
  const { subTasks } = task;

  if (subTasks?.length) {
    return (
      <TaskContentStack>
        <Box>
          <TaskFlow taskList={subTasks} />
        </Box>
        {subTasks.map((task, index) => (
          <TaskDetails key={index} task={task} bodySchema={bodySchema} />
        ))}
      </TaskContentStack>
    );
  }

  if (bodySchema?.length) {
    return (
      <div
        className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
      >
        {bodySchema.map((schema, index) => {
          const { label } = schema;
          const value = getObjectValue<unknown>(task.record, schema.accessor);

          if (schema.type === 'struct') {
            return <TaskBodyStruct key={index} label={label} value={value as object} />;
          }

          if (schema.type === 'textarea') {
            const { error, markdown } = schema as TaskBodyTextareaSchema;
            return (
              <TaskBodyTextarea
                key={index}
                label={label}
                value={value as string}
                error={error}
                markdown={markdown}
              />
            );
          }

          if (schema.type === 'metadata') {
            const { cells } = schema as TaskBodyMetadataSchema;
            return (
              <TaskBodyMetadata
                key={index}
                label={label}
                value={value as Record<string, unknown>}
                cells={cells}
              />
            );
          }

          return null;
        })}
      </div>
    );
  }

  return <div>No subtasks, no body schema for {task.name}</div>;
}
