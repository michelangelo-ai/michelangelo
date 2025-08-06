import React from 'react';
import { useStyletron } from 'baseui';

import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { TaskDetails } from './components/task-details/task-details';
import { TaskFlow } from './components/task-flow';
import { TaskSeparator } from './styled-components';
import { buildTaskList } from './utils/build-task-list';
import { buildTaskMatrix } from './utils/build-task-matrix';

import type { ExecutionDetailViewSchema } from './types';

export function Execution<
  TData extends object = object,
  TTaskRecord extends object = object,
>(props: { schema: ExecutionDetailViewSchema<TData, TTaskRecord>; data: TData }) {
  const { schema, data } = props;
  const [css, theme] = useStyletron();
  const taskList = buildTaskList(schema, data);

  if (!taskList.length) {
    return (
      <ErrorView
        illustration={
          <CircleExclamationMark
            height="64px"
            width="64px"
            kind={CircleExclamationMarkKind.PRIMARY}
          />
        }
        title={schema.emptyState.title}
        description={schema.emptyState.description}
      />
    );
  }

  const matrix = buildTaskMatrix(taskList);

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale800 })}>
      <div>
        <h3>Overview</h3>
        <div
          className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
        >
          {matrix.map((item, index) => (
            <React.Fragment key={index}>
              {index > 0 && <TaskSeparator />}
              <TaskFlow taskList={item.taskList} />
            </React.Fragment>
          ))}
        </div>
      </div>

      <div
        className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
      >
        {taskList.map((task, index) => (
          <TaskDetails
            key={index}
            task={task}
            metadata={schema.tasks.header.metadata}
            bodySchema={schema.tasks.body}
          />
        ))}
      </div>
    </div>
  );
}
