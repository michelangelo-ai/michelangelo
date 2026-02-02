import { useStyletron } from 'baseui';
import { StyledLink } from 'baseui/link';

import { Box } from '#core/components/box/box';

import type { TriggerRun } from '../types';

type TriggerInformationTabProps = {
  data: TriggerRun | undefined;
  isLoading: boolean;
};

export function TriggerInformationTab({ data, isLoading }: TriggerInformationTabProps) {
  const [css, theme] = useStyletron();

  if (isLoading) {
    return (
      <div className={css({ padding: theme.sizing.scale600 })}>
        <Box title="Information">Loading...</Box>
      </div>
    );
  }

  if (!data) {
    return (
      <div className={css({ padding: theme.sizing.scale600 })}>
        <Box title="Information">No data available</Box>
      </div>
    );
  }

  const logUrl = data.status?.logUrl;
  const errorMessage = data.status?.errorMessage;
  const parametersMap = data.spec?.trigger?.parametersMap;

  const labelStyle = css({
    ...theme.typography.LabelSmall,
    color: theme.colors.contentSecondary,
    marginBottom: theme.sizing.scale200,
  });

  const valueStyle = css({
    ...theme.typography.ParagraphSmall,
    color: theme.colors.contentPrimary,
    marginBottom: theme.sizing.scale600,
  });

  const errorStyle = css({
    ...theme.typography.ParagraphSmall,
    color: theme.colors.negative,
    backgroundColor: theme.colors.backgroundNegativeLight,
    padding: theme.sizing.scale400,
    borderRadius: theme.borders.radius200,
    marginBottom: theme.sizing.scale600,
  });

  const jsonStyle = css({
    ...theme.typography.MonoParagraphSmall,
    backgroundColor: theme.colors.backgroundSecondary,
    padding: theme.sizing.scale400,
    borderRadius: theme.borders.radius200,
    whiteSpace: 'pre-wrap',
    overflow: 'auto',
    maxHeight: '300px',
  });

  return (
    <div className={css({ padding: theme.sizing.scale600 })}>
      <Box title="Information">
        <div className={css({ display: 'flex', flexDirection: 'column' })}>
          <div className={labelStyle}>Log URL</div>
          <div className={valueStyle}>
            {logUrl ? (
              <StyledLink href={logUrl} target="_blank" rel="noopener noreferrer">
                {logUrl}
              </StyledLink>
            ) : (
              <span className={css({ color: theme.colors.contentTertiary })}>
                No log URL available
              </span>
            )}
          </div>

          {errorMessage && (
            <>
              <div className={labelStyle}>Error Message</div>
              <div className={errorStyle}>{errorMessage}</div>
            </>
          )}

          {parametersMap && Object.keys(parametersMap).length > 0 && (
            <>
              <div className={labelStyle}>Parameters</div>
              <div className={jsonStyle}>{JSON.stringify(parametersMap, null, 2)}</div>
            </>
          )}
        </div>
      </Box>
    </div>
  );
}
