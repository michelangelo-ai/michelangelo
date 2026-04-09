/**
 * @fileoverview Type/interface declarations must live in a types.ts file.
 *
 * Scattering types across component files makes them hard to find and reuse.
 * Keeping them in a co-located types.ts keeps the type surface discoverable.
 *
 * Exceptions:
 *   - Component prop interfaces (name ending in "Props") used as a function
 *     parameter type or forwardRef generic argument in the same file are
 *     allowed — they are tightly coupled to the component.
 *   - Test and fixture files are excluded via ESLint config (not this rule).
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Require type/interface declarations to live in a types.ts file',
      recommended: true,
    },
    messages: {
      typesInTypesFile: [
        "'{{ name }}' is a type declaration outside of a types.ts file.",
        'Move it to a co-located types.ts and import from there.',
        '',
        '  // types.ts',
        '  export interface {{ name }} { /* ... */ }',
        '',
        '  // component.tsx',
        "  import type { {{ name }} } from './types';",
        '',
      ].join('\n'),
    },
    schema: [],
  },

  create(context) {
    const filename = context.getPhysicalFilename?.() ?? context.filename;
    const basename = filename.split('/').pop() ?? '';

    // Allow everything inside types.ts files, files in a types/ directory, or *-types.ts files
    if (
      /^types\.[tj]sx?$/.test(basename) ||
      /[\\/]types[\\/]/.test(filename) ||
      /-types\.[tj]sx?$/.test(basename)
    ) {
      return {};
    }

    // Track all type/interface declarations and their nodes
    const declaredTypes = [];

    // Track names used as function parameter type annotations
    const paramTypeNames = new Set();

    return {
      // Collect all type/interface declarations (exported or local)
      TSInterfaceDeclaration(node) {
        const name = node.id?.name;
        if (name) {
          declaredTypes.push({ name, node });
        }
      },

      TSTypeAliasDeclaration(node) {
        const name = node.id?.name;
        if (name) {
          declaredTypes.push({ name, node });
        }
      },

      // Collect type names used in function parameter annotations
      'FunctionDeclaration > Identifier.params, FunctionDeclaration > ObjectPattern.params, ArrowFunctionExpression > Identifier.params, ArrowFunctionExpression > ObjectPattern.params, FunctionExpression > Identifier.params, FunctionExpression > ObjectPattern.params'(
        node
      ) {
        const annotation = node.typeAnnotation?.typeAnnotation;
        if (!annotation) return;

        if (annotation.type === 'TSTypeReference' && annotation.typeName?.type === 'Identifier') {
          paramTypeNames.add(annotation.typeName.name);
        }
      },

      // Collect Props types passed as generic args to forwardRef<Ref, Props>(...)
      CallExpression(node) {
        const callee = node.callee;
        if (
          callee.type !== 'Identifier' ||
          callee.name !== 'forwardRef' ||
          !node.typeArguments?.params
        ) {
          return;
        }
        for (const typeArg of node.typeArguments.params) {
          if (typeArg.type === 'TSTypeReference' && typeArg.typeName?.type === 'Identifier') {
            paramTypeNames.add(typeArg.typeName.name);
          }
        }
      },

      // At program exit, report types that aren't Props interfaces used as params
      'Program:exit'() {
        for (const { name, node } of declaredTypes) {
          if (name.endsWith('Props') && paramTypeNames.has(name)) continue;

          context.report({
            node,
            messageId: 'typesInTypesFile',
            data: { name },
          });
        }
      },
    };
  },
};

export default rule;
