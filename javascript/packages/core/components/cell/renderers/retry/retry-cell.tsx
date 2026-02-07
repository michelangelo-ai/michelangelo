import React, { useState } from 'react';
import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';
import { Modal, ModalBody, ModalFooter, ModalHeader } from 'baseui/modal';
import { create } from '@bufbuild/protobuf';
import { useQueryClient } from '@tanstack/react-query';
import { RetryInfoSchema } from '@michelangelo/rpc';

import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';

import type { CellProps } from '#core/components/cell/types';

export function RetryCell(props: CellProps<string>) {
  console.log('=== RetryCell component instantiated ===');

  const [css, theme] = useStyletron();
  const { record } = props;
  const [showRetryModal, setShowRetryModal] = useState(false);
  const [retryReason, setRetryReason] = useState('Manual retry from UI');
  const queryClient = useQueryClient();

  // Debug logging to understand the data structure
  console.log('RetryCell props:', props);
  console.log('RetryCell record:', record);
  console.log('Record state:', record?.state);
  console.log('Record activityId:', (record as any)?.activityId);

  // Get the current pipeline run context
  const { projectId, entityId } = useStudioParams('detail');

  // Fetch the complete pipeline run data
  const { data: pipelineRunData } = useStudioQuery<{ pipelineRun: Record<string, unknown> }>({
    queryName: 'GetPipelineRun',
    serviceOptions: {
      namespace: projectId,
      name: entityId,
    },
    clientOptions: {
      enabled: !!projectId && !!entityId,
    },
  });

  const updatePipelineRunMutation = useStudioMutation<
    { pipelineRun: Record<string, unknown>; updateOptions?: Record<string, unknown> },
    { pipelineRun: Record<string, unknown> }
  >({ mutationName: 'UpdatePipelineRun' });

  // Only show retry button when pipeline run is terminated AND step has activityId
  const hasActivityId = record && (record as any)?.activityId;
  const pipelineRunState = pipelineRunData?.pipelineRun?.status?.state;

  // Terminated states: SUCCEEDED=3, KILLED=4, FAILED=5, SKIPPED=6
  // Non-terminated states: PENDING=1, RUNNING=2
  const isPipelineRunTerminated = pipelineRunState && pipelineRunState >= 3;

  console.log('Pipeline run state:', pipelineRunState);
  console.log('Is pipeline run terminated:', isPipelineRunTerminated);
  console.log('Has activityId:', hasActivityId);

  if (!hasActivityId || !isPipelineRunTerminated) {
    console.log('Not showing retry button - pipeline still running or no activityId');
    return null;
  }

  const handleRetryClick = async () => {
    if (updatePipelineRunMutation.isPending || !pipelineRunData?.pipelineRun) {
      return;
    }

    const fullPipelineRun = pipelineRunData.pipelineRun;

    // Extract activityId from the step record
    const activityId = (record as any)?.activityId;
    const workflowId = (fullPipelineRun.status as any)?.workflowId;
    const workflowRunId = (fullPipelineRun.status as any)?.workflowRunId;

    // Check all required values
    console.log('Extracted values:', {
      activityId,
      workflowId,
      workflowRunId,
      retryReason
    });

    if (!activityId || !workflowId || !workflowRunId) {
      console.error('Missing required retry data:', { activityId, workflowId, workflowRunId });
      alert(`Missing required data: activityId=${activityId}, workflowId=${workflowId}, workflowRunId=${workflowRunId}`);
      return;
    }

    // Create proper protobuf RetryInfo message
    const retryInfo = create(RetryInfoSchema, {
      activityId: activityId.toString(),
      reason: retryReason,
      workflowId: workflowId,
      workflowRunId: workflowRunId,
    });

    console.log('RetryInfo protobuf message:', retryInfo);
    console.log('Current spec:', fullPipelineRun.spec);

    // Construct the updated pipeline run with retryInfo
    const updatedPipelineRun = {
      ...fullPipelineRun,
      spec: {
        ...fullPipelineRun.spec,
        retryInfo,
      },
    };

    console.log('Updated spec with retryInfo:', updatedPipelineRun.spec);
    console.log('Full updated pipeline run:', updatedPipelineRun);

    try {
      const result = await updatePipelineRunMutation.mutateAsync({
        pipelineRun: updatedPipelineRun
      });
      console.log('Retry request successful:', result);
      setShowRetryModal(false);
      setRetryReason('Manual retry from UI');

      // Refresh the pipeline run data to show updated status
      await queryClient.invalidateQueries({
        queryKey: ['GetPipelineRun', { namespace: projectId, name: entityId }],
      });

      // Show success message
      alert('Retry request submitted successfully! Page data refreshed.');
    } catch (error) {
      console.error('Failed to retry task:', error);
      alert(`Failed to retry task: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  };

  return (
    <>
      <Button
        size={SIZE.mini}
        kind={KIND.secondary}
        onClick={() => setShowRetryModal(true)}
        disabled={updatePipelineRunMutation.isPending}
      >
        Retry
      </Button>

      {/* Retry confirmation modal */}
      <Modal isOpen={showRetryModal} onClose={() => setShowRetryModal(false)}>
        <ModalHeader>Retry Task</ModalHeader>
        <ModalBody>
          <div className={css({ marginBottom: theme.sizing.scale600 })}>
            Are you sure you want to retry this task?
          </div>
          <div className={css({ marginBottom: theme.sizing.scale400 })}>
            <label className={css({ ...theme.typography.LabelMedium })}>
              Retry Reason:
            </label>
          </div>
          <textarea
            value={retryReason}
            onChange={(e) => setRetryReason(e.target.value)}
            placeholder="Enter reason for retry..."
            className={css({
              width: '100%',
              padding: theme.sizing.scale400,
              border: `1px solid ${theme.colors.borderOpaque}`,
              borderRadius: theme.borders.radius200,
              resize: 'vertical',
              minHeight: '80px',
              fontFamily: 'inherit',
            })}
          />
        </ModalBody>
        <ModalFooter>
          <div className={css({ display: 'flex', gap: theme.sizing.scale400 })}>
            <Button
              size={SIZE.compact}
              kind={KIND.secondary}
              onClick={() => setShowRetryModal(false)}
              disabled={updatePipelineRunMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              size={SIZE.compact}
              kind={KIND.primary}
              onClick={handleRetryClick}
              isLoading={updatePipelineRunMutation.isPending}
            >
              Retry Task
            </Button>
          </div>
        </ModalFooter>
      </Modal>
    </>
  );
}