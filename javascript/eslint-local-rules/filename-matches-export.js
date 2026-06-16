/**
 * @fileoverview Enforces that a .tsx file's primary component export matches its filename.
 *
 * A file named `button-group.tsx` should export `ButtonGroup`. Drift between
 * filename and export name makes grep and autocomplete unreliable in a component
 * library where the filename is the canonical identifier.
 */

const kebabToPascal = (str) =>
  str
    .split('-')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join('');

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Enforce that a .tsx file exports a component whose name matches the filename stem in PascalCase',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/filename-matches-export.md',
    },
    messages: {
      filenameMismatch:
        "'{{filename}}' must export a component named '{{expectedName}}'. " +
        'Rename the export or the file so they match.',
    },
    schema: [],
  },

  create(context) {
    const filename = context.getPhysicalFilename?.() ?? context.filename;
    const basename = filename.split('/').pop() ?? '';

    // Only apply to .tsx files
    if (!basename.endsWith('.tsx')) return {};

    const stem = basename.slice(0, -4);

    // Skip entry points and styled-component collections
    if (stem === 'index' || stem.includes('styled')) return {};

    // Skip files that don't follow kebab-case (e.g. single-word all-lowercase utility files)
    // A valid component file stem must produce a meaningful PascalCase name
    const expectedName = kebabToPascal(stem);

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
        // Only flag files that have at least one PascalCase export — these are component files.
        // ALL_CAPS constants and lowercase utilities are exempt. Requires uppercase first char
        // followed by at least one lowercase char to distinguish PascalCase from ALL_CAPS.
        const hasPascalExport = [...exportedNames].some((name) => /^[A-Z][a-z]/.test(name));
        if (!hasPascalExport) return;

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
