import { useStyletron } from 'baseui';
import { Skeleton } from 'baseui/skeleton';
import { HeadingSmall } from 'baseui/typography';

import { Box } from '#core/components/box/box';
import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { Form } from '#core/components/form/form';
import { LinksBox } from '#core/components/links-box/links-box';
import { TextEditor } from '#core/components/text-editor/text-editor';

import type { TriggerRun } from './types';

/**
 * "Information" tab for a Trigger's detail page: the run's log link (if any), its error
 * message (if it failed), and the raw pipeline-execution parameters it ran with.
 *
 * Custom detail pages receive the entity as a plain object (see {@link CustomDetailPageConfig});
 * narrowing to {@link TriggerRun} here for property access.
 */
export function TriggerInfoPage({ data, isLoading }: { data?: object; isLoading: boolean }) {
  const [css, theme] = useStyletron();
  // cast: custom detail pages receive the entity as a plain object; narrowing to the
  // expected proto shape for property access; see #1425
  const trigger = data as TriggerRun | undefined;

  const errorMessage = trigger?.status?.errorMessage;
  const parametersMap = trigger?.spec?.trigger?.parametersMap;
  const hasParameters = parametersMap !== undefined && Object.keys(parametersMap).length > 0;

  const fieldColumn = css({
    display: 'flex',
    flexDirection: 'column',
    gap: theme.sizing.scale600,
  });

  return (
    <div className={fieldColumn}>
      <LinksBox
        title="Useful links"
        isLoading={isLoading}
        links={[{ name: 'Trigger run logs', url: trigger?.status?.logUrl }]}
      />

      {errorMessage && !isLoading && (
        <section>
          <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
            Message
          </HeadingSmall>
          <Box>
            <Form onSubmit={() => undefined} initialValues={{ 'error-message': errorMessage }}>
              <TextareaField name="error-message" label="Error message" readOnly />
            </Form>
          </Box>
        </section>
      )}

      {hasParameters &&
        (isLoading ? (
          <Skeleton animation height="200px" width="100%" />
        ) : (
          <section>
            <HeadingSmall marginTop="0" marginBottom={theme.sizing.scale600}>
              Parameters
            </HeadingSmall>
            <Box>
              <TextEditor
                value={JSON.stringify(parametersMap, null, 2)}
                language="json"
                readOnly
                foldable
                height="auto"
              />
            </Box>
          </section>
        ))}
    </div>
  );
}
