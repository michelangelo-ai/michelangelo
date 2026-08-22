import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall, ParagraphMedium } from 'baseui/typography';

import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { ChartRecord, EvaluationReportRecord, ModelRecord } from './types';

/**
 * Performance tab for the Model detail page.
 *
 * Resolves the model's `performanceEvaluationReport` reference to its `EvaluationReport`
 * resource and lists the charts it contains by title. Rendering a chart's actual content (as a
 * table, line chart, histogram, etc.) is not yet implemented — that depends on a per-chart-type
 * rendering approach, and for non-table chart types, a charting library, both still to be
 * decided. This tab proves out the fetch/loading/empty-state plumbing ahead of that work.
 */
export function ModelPerformancePage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();

  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const model = data as ModelRecord | undefined;
  const reportName = model?.spec?.performanceEvaluationReport?.name;

  const { data: reportData, isLoading: isReportLoading } = useStudioQuery<{
    evaluationReport?: EvaluationReportRecord;
  }>({
    queryName: 'GetEvaluationReport',
    serviceOptions: { name: reportName },
    clientOptions: { enabled: !isLoading && Boolean(reportName) },
  });

  if (isLoading || isReportLoading) {
    return (
      <div
        className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}
      >
        <Skeleton animation height="24px" width="240px" />
        <Skeleton animation height="80px" width="100%" />
      </div>
    );
  }

  if (!reportName) {
    return (
      <ParagraphMedium margin="0">
        No performance report is available for this model.
      </ParagraphMedium>
    );
  }

  const report = reportData?.evaluationReport;
  const charts: ChartRecord[] = report?.spec?.charts ?? [];

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale600 })}>
      {report?.spec?.title && (
        <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
          {report.spec.title}
        </HeadingSmall>
      )}
      {charts.length === 0 ? (
        <ParagraphMedium margin="0">This report has no charts.</ParagraphMedium>
      ) : (
        <ul className={css({ margin: 0, paddingLeft: theme.sizing.scale600 })}>
          {charts.map((chart, index) => (
            <li key={chart.title ?? index}>
              <ParagraphMedium margin="0">{chart.title ?? 'Untitled chart'}</ParagraphMedium>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
