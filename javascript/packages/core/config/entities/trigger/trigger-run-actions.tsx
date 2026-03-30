import React from 'react';
import { useStyletron } from 'baseui';

import { TriggerRunActionDialog } from './trigger-run-action-dialog';
import { TriggerRunState } from './types';

import type { TriggerRun } from './types';

export interface TriggerRunActionsProps {
  record: TriggerRun;
}

export const TriggerRunActions: React.FC<TriggerRunActionsProps> = ({ record }) => {
  const [css, theme] = useStyletron();

  if (!record?.status?.state) {
    return null;
  }

  const currentState = record.status.state;

  return (
    <div className={css({ display: 'flex', gap: theme.sizing.scale300 })}>
      {/* Show kill button for running triggers */}
      {currentState === TriggerRunState.RUNNING && (
        <TriggerRunActionDialog record={record} action="kill" />
      )}

      {/* Show pause button for running cron/interval triggers */}
      {currentState === TriggerRunState.RUNNING && (
        <TriggerRunActionDialog record={record} action="pause" />
      )}

      {/* Show resume button for paused triggers */}
      {currentState === TriggerRunState.PAUSED && (
        <TriggerRunActionDialog record={record} action="resume" />
      )}
    </div>
  );
};