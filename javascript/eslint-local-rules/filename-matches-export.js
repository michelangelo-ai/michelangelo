/**
 * @fileoverview Enforces that a component or hook file's primary export matches its filename.
 *
 * A file named `button-group.tsx` should export `ButtonGroup`.
 * A file named `use-studio-mutation.ts` should export `useStudioMutation`.
 * Drift between filename and export makes grep and autocomplete unreliable in a
 * component library where the filename is the canonical identifier.
 */

const kebabToPascal = (str) =>
  str
    .split('-')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join('');

// use-studio-mutation → useStudioMutation
const kebabToCamel = (str) => {
  const parts = str.split('-');
  return (
    parts[0] +
    parts
      .slice(1)
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join('')
  );
};

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Enforce that a component or hook file exports a name matching the filename stem',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/filename-matches-export.md',
    },
    messages: {
      filenameMismatch:
        "'{{filename}}' must export a component or hook named '{{expectedName}}'. " +
        'Rename the export or the file so they match.',
    },
    schema: [],
  },

  create(context) {
    const filename = context.getPhysicalFilename?.() ?? context.filename;
    const basename = filename.split('/').pop() ?? '';

    const isTsx = basename.endsWith('.tsx');
    const isTs = basename.endsWith('.ts');
    if (!isTsx && !isTs) return {};

    const stem = isTsx ? basename.slice(0, -4) : basename.slice(0, -3);

    // Skip entry points and styled-component collections
    if (stem === 'index' || stem.includes('styled')) return {};

    const isHookFile = stem.startsWith('use-');

    let expectedName;
    if (isHookFile) {
      expectedName = kebabToCamel(stem);
    } else if (isTsx) {
      expectedName = kebabToPascal(stem);
    } else {
      return {}; // Non-hook .ts files (utilities, types) — skip
    }

    const exportedNames = new Set();

    return {
      ExportNamedDeclaration(node) {
        // Skip type-only exports
        if (node.exportKind === 'type') return;

        // export function ComponentName / export function* gen
        if (node.declaration?.type === 'FunctionDeclaration' && node.declaration.id) {
          exportedNames.add(node.declaration.id.name);
        }

        // export const ComponentName = ... / export const A = ..., B = ...
        if (node.declaration?.type === 'VariableDeclaration') {
          for (const declarator of node.declaration.declarations) {
            if (declarator.id?.type === 'Identifier') {
              exportedNames.add(declarator.id.name);
            }
          }
        }

        // export class ComponentName
        if (node.declaration?.type === 'ClassDeclaration' && node.declaration.id) {
          exportedNames.add(node.declaration.id.name);
        }
      },

      'Program:exit'(node) {
        if (isHookFile) {
          // Only flag files that export at least one hook (use + uppercase letter)
          const hasHookExport = [...exportedNames].some((name) => /^use[A-Z]/.test(name));
          if (!hasHookExport) return;
        } else {
          // Only flag files that have at least one PascalCase export — these are component files.
          // ALL_CAPS constants and lowercase utilities are exempt.
          const hasPascalExport = [...exportedNames].some((name) => /^[A-Z][a-z]/.test(name));
          if (!hasPascalExport) return;
        }

        if (!exportedNames.has(expectedName)) {
          context.report({
            node,
            messageId: 'filenameMismatch',
            data: { expectedName, filename: basename },
          });
        }
      },
    };
  },
};

export default rule;
