import React from 'react';
import { Block } from 'baseui/block';
import { LabelSmall, ParagraphMedium } from 'baseui/typography';

interface DeploymentStatusChartProps {
  deployments: any[];
}

export function DeploymentStatusChart({ deployments }: DeploymentStatusChartProps) {
  // Mock data for demonstration - replace with actual deployment status logic
  const statusData = [
    { status: 'Healthy', count: 3, color: '#00B44A' },
    { status: 'Deploying', count: 1, color: '#F59E0B' },
    { status: 'Failed', count: 0, color: '#EF4444' },
    { status: 'Retired', count: 2, color: '#6B7280' },
  ];

  const total = statusData.reduce((sum, item) => sum + item.count, 0);

  return (
    <Block>
      {/* Simple status bars */}
      <Block marginBottom="scale600">
        {statusData.map((item, index) => (
          <Block key={index} marginBottom="scale300">
            <Block display="flex" justifyContent="space-between" alignItems="center" marginBottom="scale100">
              <LabelSmall>{item.status}</LabelSmall>
              <LabelSmall>{item.count}</LabelSmall>
            </Block>
            <Block
              height="8px"
              backgroundColor="#F1F5F9"
              borderRadius="4px"
              overflow="hidden"
            >
              <Block
                height="100%"
                width={total > 0 ? `${(item.count / total) * 100}%` : '0%'}
                backgroundColor={item.color}
                $style={{
                  transition: 'width 0.3s ease-in-out',
                }}
              />
            </Block>
          </Block>
        ))}
      </Block>
      
      {/* Summary */}
      <Block 
        padding="scale400" 
        backgroundColor="#F8FAFC" 
        borderRadius="8px"
      >
        <ParagraphMedium margin="0" color="contentSecondary">
          Total: {total} deployments across all environments
        </ParagraphMedium>
      </Block>
    </Block>
  );
}