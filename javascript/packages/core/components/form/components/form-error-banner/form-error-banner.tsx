import { useStyletron } from 'baseui';
import { StyledLink } from 'baseui/link';

import { Banner } from '#core/components/banner/banner';
import { Markdown } from '#core/components/markdown/markdown';
import { useFormErrorList } from './use-form-error-list';

import type { ErrorEntry } from './types';

/** Renders a banner containing form submission errors and validation errors
 * with clickable links to the fields that have errors.
 *
 * Supports Markdown for the error message.
 *
 * @requires Must be rendered within a `<Form>` component
 */
export function FormErrorBanner() {
  const [css, theme] = useStyletron();
  const errors = useFormErrorList();

  if (errors.length === 0) return null;

  return (
    <Banner kind="negative">
      {errors.map((entry: ErrorEntry) => (
        <div
          key={entry.fieldPath}
          className={css({ ...theme.typography.ParagraphSmall, textAlign: 'left' })}
        >
          {entry.fieldLabel && (
            <StyledLink
              $as="button"
              type="button"
              onClick={entry.focus}
              className={css({
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: '0',
              })}
            >
              {entry.fieldLabel}
            </StyledLink>
          )}
          {entry.fieldLabel && entry.errorMessage && <>&nbsp;</>}
          <Markdown>{entry.errorMessage}</Markdown>
        </div>
      ))}
    </Banner>
  );
}
