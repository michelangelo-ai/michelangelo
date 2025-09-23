import { useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { TaskContentStack } from '#core/components/views/execution/styled-components';
import { buildTaskMatrix } from '#core/components/views/execution/utils/build-task-matrix';
import { scrollToTask } from '#core/components/views/execution/utils/scroll-to-task';
import { useInterpolationResolver } from '#core/interpolation/use-interpolation-resolver';
import { getObjectValue } from '#core/utils/object-utils';
import { TaskFlow } from '../task-flow';
import { TaskBodyMetadata } from './renderers/task-body-metadata';
import { TaskBodyStruct } from './renderers/task-body-struct';
import { TaskBodyTextarea } from './renderers/task-body-textarea';
import { TaskDetails } from './task-details';

import type { TaskBodyProps } from './types';

export function TaskBody<TTaskRecord extends object>(props: TaskBodyProps<TTaskRecord>) {
  const [css, theme] = useStyletron();
  const { task, bodySchema, overrides, metadata } = props;
  const { subTasks } = task;
  const resolver = useInterpolationResolver();

  if (subTasks?.length) {
    const matrix = buildTaskMatrix(subTasks);
    return (
      <TaskContentStack>
        <Box>
          <TaskFlow
            matrix={matrix}
            onTaskClick={scrollToTask}
            overrides={{ TaskListRenderer: overrides?.TaskListRenderer }}
          />
        </Box>
        {subTasks.map((task, index) => (
          <TaskDetails
            key={index}
            task={task}
            bodySchema={bodySchema}
            overrides={overrides}
            metadata={metadata}
          />
        ))}
      </TaskContentStack>
    );
  }

  if (bodySchema?.length) {
    const resolvedBodySchema = resolver(bodySchema, { row: task });
    return (
      <div
        className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
      >
        {resolvedBodySchema.map((schema, index) => {
          const { label } = schema;

          if (schema.type === 'struct') {
            const value = getObjectValue<object>(task.record, schema.accessor);
            return <TaskBodyStruct key={index} label={label} value={value} />;
          }

          if (schema.type === 'textarea') {
            const value = getObjectValue<string>(task.record, schema.accessor);
            const { error, markdown } = schema;
            return (
              <TaskBodyTextarea
                key={index}
                label={label}
                value={value}
                error={error}
                markdown={markdown}
              />
            );
          }

          if (schema.type === 'metadata') {
            const { cells } = schema;
            return (
              <TaskBodyMetadata
                key={index}
                label={label}
                value={task.record as Record<string, unknown>}
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
