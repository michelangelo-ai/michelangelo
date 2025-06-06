import React from 'react';
import { Block } from 'baseui/block';
import { ParagraphMedium, LabelSmall } from 'baseui/typography';
import { Tag } from 'baseui/tag';
import { Button } from 'baseui/button';

export function AlertsList() {
  const alerts = [
    {
      id: 1,
      severity: 'warning',
      title: 'High storage usage detected',
      description: 'Storage usage has exceeded 85% threshold',
      component: 'Storage',
      timestamp: '2025-01-15T14:30:00Z',
      acknowledged: false
    },
    {
      id: 2,
      severity: 'info',
      title: 'Model deployment completed',
      description: 'BERT-CoLA model successfully deployed to production',
      component: 'Model Serving',
      timestamp: '2025-01-15T13:45:00Z',
      acknowledged: true
    },
    {
      id: 3,
      severity: 'error',
      title: 'Deployment failed',
      description: 'Image classifier deployment failed due to resource constraints',
      component: 'Kubernetes',
      timestamp: '2025-01-15T12:20:00Z',
      acknowledged: false
    },
    {
      id: 4,
      severity: 'warning',
      title: 'High latency detected',
      description: 'Average response time increased to 250ms',
      component: 'API Server',
      timestamp: '2025-01-15T11:15:00Z',
      acknowledged: true
    }
  ];

  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'error': return 'negative';
      case 'warning': return 'warning';
      case 'info': return 'primary';
      default: return 'neutral';
    }
  };

  const getSeverityIcon = (severity: string) => {
    switch (severity) {
      case 'error': return '🚨';
      case 'warning': return '⚠️';
      case 'info': return 'ℹ️';
      default: return '📋';
    }
  };

  const formatTimestamp = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diffInMinutes = Math.floor((now.getTime() - date.getTime()) / (1000 * 60));
    
    if (diffInMinutes < 60) {
      return `${diffInMinutes} minutes ago`;
    } else if (diffInMinutes < 1440) {
      return `${Math.floor(diffInMinutes / 60)} hours ago`;
    } else {
      return date.toLocaleDateString();
    }
  };

  return (
    <Block>
      {alerts.map((alert) => (
        <Block
          key={alert.id}
          padding="scale600"
          marginBottom="scale400"
          backgroundColor={alert.acknowledged ? '#F8FAFC' : '#FEF7F0'}
          borderRadius="8px"
          border={`1px solid ${alert.acknowledged ? '#E2E8F0' : '#FED7AA'}`}
          $style={{
            opacity: alert.acknowledged ? 0.7 : 1,
          }}
        >
          <Block display="flex" justifyContent="space-between" alignItems="flex-start">
            <Block flex="1">
              <Block display="flex" alignItems="center" marginBottom="scale300">
                <Block marginRight="scale300" fontSize="16px">
                  {getSeverityIcon(alert.severity)}
                </Block>
                <ParagraphMedium margin="0" font="font600" marginRight="scale400">
                  {alert.title}
                </ParagraphMedium>
                <Tag
                  kind={getSeverityColor(alert.severity)}
                  closeable={false}
                  size="small"
                >
                  {alert.severity.toUpperCase()}
                </Tag>
              </Block>
              
              <ParagraphMedium margin="0" marginBottom="scale300" color="contentSecondary">
                {alert.description}
              </ParagraphMedium>
              
              <Block display="flex" alignItems="center" gridGap="scale400">
                <LabelSmall color="contentSecondary">
                  Component: {alert.component}
                </LabelSmall>
                <LabelSmall color="contentSecondary">
                  {formatTimestamp(alert.timestamp)}
                </LabelSmall>
                {alert.acknowledged && (
                  <Tag kind="positive" closeable={false} size="small">
                    ACKNOWLEDGED
                  </Tag>
                )}
              </Block>
            </Block>
            
            <Block display="flex" gridGap="scale200">
              {!alert.acknowledged && (
                <Button size="mini" kind="secondary">
                  Acknowledge
                </Button>
              )}
              <Button size="mini" kind="tertiary">
                View Details
              </Button>
            </Block>
          </Block>
        </Block>
      ))}
      
      {alerts.length === 0 && (
        <Block
          padding="scale800"
          textAlign="center"
          backgroundColor="#F0FDF4"
          borderRadius="8px"
          border="1px solid #BBF7D0"
        >
          <Block fontSize="24px" marginBottom="scale400">✅</Block>
          <ParagraphMedium margin="0" color="positive">
            No active alerts. All systems are running smoothly!
          </ParagraphMedium>
        </Block>
      )}
    </Block>
  );
}