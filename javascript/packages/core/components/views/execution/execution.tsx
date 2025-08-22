import { getOverrides, useStyletron } from 'baseui';

import { Box } from '#core/components/box/box';
import { ErrorView } from '#core/components/error-view/error-view';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { TaskDetails } from './components/task-details/task-details';
import { TaskFlow } from './components/task-flow';
import { TaskStateIcon } from './components/task-state-icon';
import { TaskContentStack } from './styled-components';
import { buildTaskList } from './utils/build-task-list';
import { buildTaskMatrix } from './utils/build-task-matrix';
import { determineExecutionState } from './utils/determine-execution-state';
import { scrollToTask } from './utils/scroll-to-task';

import type { Task } from './types';
import type { ExecutionDetailViewSchema, ExecutionOverrides } from './types';

export function Execution<
  TData extends object = object,
  TTaskRecord extends object = object,
>(props: {
  schema: ExecutionDetailViewSchema<TData, TTaskRecord>;
  data: TData;
  overrides?: ExecutionOverrides<TTaskRecord>;
}) {
  const { schema, data, overrides } = props;
  const [css, theme] = useStyletron();
  const taskList = overrides?.taskList ?? buildTaskList(schema, data);

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

  const [TaskFlowComponent, taskFlowProps] = getOverrides(overrides?.TaskFlow, TaskFlow);

  return (
    <TaskContentStack>
      <Box
        title={
          <div
            className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale500 })}
          >
            <TaskStateIcon state={determineExecutionState(taskList)} />
            Overview
          </div>
        }
      >
        <TaskContentStack>
          <TaskFlowComponent
            matrix={matrix}
            onTaskClick={(clickedTask) => {
              scrollToTask(clickedTask as Task<TTaskRecord>);
            }}
            {...taskFlowProps}
          />
        </TaskContentStack>
      </Box>

      <TaskContentStack>
        {taskList.map((task, index) => (
          <TaskDetails
            key={index}
            task={task}
            metadata={schema.tasks.header.metadata}
            bodySchema={schema.tasks.body}
            overrides={overrides}
          />
        ))}
      </TaskContentStack>
    </TaskContentStack>
  );
}
