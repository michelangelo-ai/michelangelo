import { useStyletron } from 'baseui';
import { Breadcrumbs } from 'baseui/breadcrumbs';
import { Cell, Grid } from 'baseui/layout-grid';

import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { Phase } from '#core/types/common/studio-types';
import { MenuDrawer } from './menu-drawer';
import { BreadcrumbContainer, PlainLink } from './styled-components';

import type { CategoryConfig } from '#core/types/common/studio-types';

/**
 * A breadcrumb navigation with an integrated hamburger menu drawer.
 *
 * Reads the current URL to build the breadcrumb trail:
 * Home > Project > Category > Phase > Entity > EntityId
 *
 * Category and Phase segments are derived from the supplied categories config.
 * The menu drawer shows all phases from all supplied categories.
 *
 * @example
 * ```tsx
 * const CATEGORIES: CategoryConfig[] = [
 *   { id: 'core-ml', name: 'Core ML', phases: Object.values(PHASES) },
 * ];
 * <BreadcrumbBar categories={CATEGORIES} />
 * ```
 */
export function BreadcrumbBar({ categories }: { categories: CategoryConfig[] }) {
  const [css, theme] = useStyletron();
  const { projectId, phase, entityId } = useStudioParams('base');
  const isProjectPage = phase === Phase.Project;
  const allPhases = categories.flatMap((c) => c.phases);

  return (
    <BreadcrumbContainer>
      <Grid gridColumns={1} gridGutters={0} gridGaps={0}>
        <Cell>
          <div
            className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale600 })}
          >
            <MenuDrawer phases={allPhases} projectId={projectId} />
            <Breadcrumbs
              overrides={{
                Root: {
                  style: { color: theme.colors.contentPrimary },
                  props: { 'data-tracking-name': 'breadcrumb-bar' },
                },
              }}
            >
              <PlainLink to="/" data-tracking-name="home">
                Home
              </PlainLink>
              <ProjectBreadcrumb />
              {!isProjectPage && <CategoryBreadcrumb categories={categories} />}
              {!isProjectPage && <PhaseBreadcrumb categories={categories} />}
              {!isProjectPage && <EntityBreadcrumb categories={categories} />}
              {!isProjectPage && entityId && <EntityIdBreadcrumb />}
            </Breadcrumbs>
          </div>
        </Cell>
      </Grid>
    </BreadcrumbContainer>
  );
}

function ProjectBreadcrumb() {
  const { projectId, phase } = useStudioParams('base');
  const isProjectPage = phase === Phase.Project;
  return isProjectPage ? (
    <span>{projectId}</span>
  ) : (
    <PlainLink to={`/${projectId}`} data-tracking-name="project">
      {projectId}
    </PlainLink>
  );
}

function CategoryBreadcrumb({ categories }: { categories: CategoryConfig[] }) {
  const { projectId, phase } = useStudioParams('base');
  // TODO #909: Deprecate Phase enum in favor of string, since Phase is configurable at
  // the app configuration level
  // eslint-disable-next-line @typescript-eslint/no-unsafe-enum-comparison
  const category = categories.find((c) => c.phases.some((p) => p.id === phase));
  if (!category) return null;
  return (
    <PlainLink to={`/${projectId}`} data-tracking-name="category">
      {category.name}
    </PlainLink>
  );
}

function PhaseBreadcrumb({ categories }: { categories: CategoryConfig[] }) {
  const { projectId, phase } = useStudioParams('base');
  // TODO #909
  // eslint-disable-next-line @typescript-eslint/no-unsafe-enum-comparison
  const currentPhase = categories.flatMap((c) => c.phases).find((p) => p.id === phase);
  const phaseName = currentPhase?.name ?? phase;
  return (
    <PlainLink to={`/${projectId}/${phase}`} data-tracking-name="phase">
      {phaseName}
    </PlainLink>
  );
}

function EntityBreadcrumb({ categories }: { categories: CategoryConfig[] }) {
  const { projectId, phase, entity, entityId } = useStudioParams('base');
  // TODO #909
  // eslint-disable-next-line @typescript-eslint/no-unsafe-enum-comparison
  const currentPhase = categories.flatMap((c) => c.phases).find((p) => p.id === phase);
  const entityName = currentPhase?.entities.find((e) => e.id === entity)?.name ?? entity;

  return entityId ? (
    <PlainLink to={`/${projectId}/${phase}/${entity}`} data-tracking-name="entity">
      {entityName}
    </PlainLink>
  ) : (
    <span>{entityName}</span>
  );
}

function EntityIdBreadcrumb() {
  const { entityId } = useStudioParams('form-detail');
  return <span>{entityId}</span>;
}
