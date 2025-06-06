import React from 'react';
import { Card } from 'baseui/card';
import { Block } from 'baseui/block';
import { HeadingSmall, ParagraphMedium, LabelSmall } from 'baseui/typography';

interface StatCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  trend?: string;
  color?: string;
}

export function StatCard({ title, value, subtitle, trend, color = '#1B7CFC' }: StatCardProps) {
  return (
    <Card
      overrides={{
        Root: {
          style: {
            border: `1px solid #E2E8F0`,
            borderLeft: `4px solid ${color}`,
          }
        }
      }}
    >
      <Block>
        <LabelSmall color="contentSecondary" marginBottom="scale200">
          {title}
        </LabelSmall>
        <HeadingSmall margin="0" marginBottom="scale200">
          {value}
        </HeadingSmall>
        {subtitle && (
          <ParagraphMedium 
            margin="0" 
            color="contentSecondary"
            marginBottom="scale100"
          >
            {subtitle}
          </ParagraphMedium>
        )}
        {trend && (
          <LabelSmall 
            color={trend.includes('+') || trend.includes('100%') ? 'positive' : 'contentSecondary'}
          >
            {trend}
          </LabelSmall>
        )}
      </Block>
    </Card>
  );
}