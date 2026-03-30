import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';

import { Dialog } from '#core/components/dialog/dialog';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { TriggerRunAction, TriggerRunState } from '#core/config/entities/trigger/types';

import type { CellRendererProps } from '#core/components/cell/types';
import type { TriggerRun } from '#core/config/entities/trigger/types';

export interface TriggerActionCellProps extends CellRendererProps<TriggerRun> {
  action: 'kill' | 'pause' | 'resume';
}

const ACTION_CONFIG = {
  kill: {
    label: 'Kill',
    heading: 'Kill Trigger Run',
    description: 'This will terminate the trigger run and any associated pipeline runs.',
    confirmText: 'Are you sure you want to kill this trigger run?',
    buttonKind: KIND.primary,
    submitLabel: 'Kill',
    // Show kill button for running triggers
    shouldShow: (state: TriggerRunState) => state === TriggerRunState.RUNNING,
  },
  pause: {
    label: 'Pause',
    heading: 'Pause Trigger Run',
    description: 'This will pause the recurring trigger to prevent new executions.',
    confirmText: 'Are you sure you want to pause this recurring trigger?',
    buttonKind: KIND.secondary,
    submitLabel: 'Pause',
    // Show pause button for running cron/interval triggers
    shouldShow: (state: TriggerRunState) => state === TriggerRunState.RUNNING,
  },
  resume: {
    label: 'Resume',
    heading: 'Resume Trigger Run',
    description: 'This will resume the paused trigger to allow new executions.',
    confirmText: 'Are you sure you want to resume this trigger?',
    buttonKind: KIND.secondary,
    submitLabel: 'Resume',
    // Show resume button for paused triggers
    shouldShow: (state: TriggerRunState) => state === TriggerRunState.PAUSED,
  },
} as const;

export const TriggerActionCell = (props: TriggerActionCellProps) => {
  const { value: triggerRun, action } = props;
  const [css, theme] = useStyletron();
  const [showModal, setShowModal] = useState(false);
  const queryClient = useQueryClient();

  const { projectId } = useStudioParams('base');

  const config = ACTION_CONFIG[action];

  const updateTriggerRunMutation = useStudioMutation<
    { triggerRun: TriggerRun },
    { triggerRun: TriggerRun }
  >({ mutationName: 'UpdateTriggerRun' });

  if (!triggerRun?.status?.state) {
    return null;
  }

  const currentState = triggerRun.status.state;

  // Check if this action should be shown for the current state
  if (!config.shouldShow(currentState)) {
    return null;
  }

  const submitAction = async () => {
    if (updateTriggerRunMutation.isPending || !triggerRun) {
      return;
    }

    // Create updated trigger run with the action
    const updatedTriggerRun: TriggerRun = {
      ...triggerRun,
      spec: {
        ...triggerRun.spec,
        // Use only the action field (clean approach, deprecated boolean fields not set)
        action: action === 'kill' ? TriggerRunAction.KILL :
                action === 'pause' ? TriggerRunAction.PAUSE :
                TriggerRunAction.RESUME,
      },
    };

    try {
      await updateTriggerRunMutation.mutateAsync({ triggerRun: updatedTriggerRun });
      setShowModal(false);

      // Invalidate queries to refresh the UI
      await queryClient.invalidateQueries({
        queryKey: ['ListTriggerRun', { namespace: projectId }],
      });

      if (triggerRun.metadata?.name) {
        await queryClient.invalidateQueries({
          queryKey: ['GetTriggerRun', { namespace: projectId, name: triggerRun.metadata.name }],
        });
      }
    } catch {
      // Error is captured in updateTriggerRunMutation.error and displayed in the modal
    }
  };

  return (
    <>
      <Button
        size={SIZE.mini}
        kind={config.buttonKind}
        onClick={() => setShowModal(true)}
        disabled={updateTriggerRunMutation.isPending}
      >
        {config.label}
      </Button>

      <Dialog
        isOpen={showModal}
        onDismiss={() => setShowModal(false)}
        heading={config.heading}
        buttonDock={{
          primaryAction: (
            <Button
              kind={config.buttonKind}
              onClick={submitAction}
              isLoading={updateTriggerRunMutation.isPending}
            >
              {config.submitLabel}
            </Button>
          ),
          dismissiveAction: (
            <Button
              kind={KIND.tertiary}
              onClick={() => setShowModal(false)}
              disabled={updateTriggerRunMutation.isPending}
            >
              Cancel
            </Button>
          ),
        }}
      >
        {updateTriggerRunMutation.error && (
          <div
            className={css({
              color: theme.colors.negative,
              marginBottom: theme.sizing.scale600,
              ...theme.typography.ParagraphSmall,
            })}
          >
            {updateTriggerRunMutation.error.message}
          </div>
        )}
        <div className={css({ marginBottom: theme.sizing.scale600 })}>
          {config.confirmText}
        </div>
        <div className={css({
          marginBottom: theme.sizing.scale600,
          ...theme.typography.ParagraphXSmall,
          color: theme.colors.contentTertiary,
        })}>
          {config.description}
        </div>
        <div className={css({
          ...theme.typography.LabelSmall,
          marginBottom: theme.sizing.scale400,
        })}>
          Trigger Run: {triggerRun.metadata?.name}
        </div>
      </Dialog>
    </>
  );
};

// Export individual action cells for easier use
export const KillTriggerCell = (props: CellRendererProps<TriggerRun>) => (
  <TriggerActionCell {...props} action="kill" />
);

export const PauseTriggerCell = (props: CellRendererProps<TriggerRun>) => (
  <TriggerActionCell {...props} action="pause" />
);

export const ResumeTriggerCell = (props: CellRendererProps<TriggerRun>) => (
  <TriggerActionCell {...props} action="resume" />
);