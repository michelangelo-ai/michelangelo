/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Require type/interface declarations to live in a types.ts file',
      recommended: true,
    },
    messages: {
      inlineType: [
        "'{{ name }}' is a small, single-use type outside of a types.ts file.",
        'If the name adds clarity, move it to a co-located types.ts.',
        'Otherwise, consider inlining it at the call site.',
        '',
        '  // Inline',
        '  function foo(arg: { ... }) { ... }',
        '',
        '  // Or move to types.ts',
        "  import type { {{ name }} } from './types';",
      ].join('\n'),
      typesInTypesFile: [
        "'{{ name }}' is a type declaration outside of a types.ts file.",
        'Move it to a co-located types.ts and import from there.',
        '',
        '  // types.ts',
        '  export interface {{ name }} { /* ... */ }',
        '',
        '  // component.tsx',
        "  import type { {{ name }} } from './types';",
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

    const declaredTypes = [];
    const paramTypeNames = new Set();
    const countTypeRefs = new Map();

    function isSmall(node) {
      if (node.type === 'TSTypeAliasDeclaration') {
        const t = node.typeAnnotation;
        if (t.type === 'TSTypeLiteral') return t.members.length <= 2;
        if (t.type === 'TSUnionType') return t.types.length <= 2;
        return false;
      }
      if (node.type === 'TSInterfaceDeclaration') {
        return node.body.body.length <= 2;
      }
      return false;
    }

    return {
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

      'FunctionDeclaration > Identifier.params, FunctionDeclaration > ObjectPattern.params, ArrowFunctionExpression > Identifier.params, ArrowFunctionExpression > ObjectPattern.params, FunctionExpression > Identifier.params, FunctionExpression > ObjectPattern.params'(
        node
      ) {
        const annotation = node.typeAnnotation?.typeAnnotation;
        if (!annotation) return;

        if (annotation.type === 'TSTypeReference' && annotation.typeName?.type === 'Identifier') {
          paramTypeNames.add(annotation.typeName.name);
        }
      },

      TSTypeReference(node) {
        if (node.typeName?.type === 'Identifier') {
          const name = node.typeName.name;
          countTypeRefs.set(name, (countTypeRefs.get(name) ?? 0) + 1);
        }
      },

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

      'Program:exit'() {
        for (const { name, node } of declaredTypes) {
          if (name.endsWith('Props') && paramTypeNames.has(name)) continue;

          const isSingleUse = (countTypeRefs.get(name) ?? 0) === 1;
          const small = isSmall(node);

          context.report({
            node,
            messageId: isSingleUse && small ? 'inlineType' : 'typesInTypesFile',
            data: { name },
          });
        }
      },
    };
  },
};

export default rule;
