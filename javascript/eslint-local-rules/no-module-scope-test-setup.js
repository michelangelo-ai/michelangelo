/**
 * @fileoverview Disallows module-level variable declarations used for test setup.
 *
 * Wrappers, props, and component configurations defined at the top of a test
 * file accumulate invisible shared state across tests, making failures harder
 * to reason about. Inline everything inside each test instead.
 */

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Disallow module-level variable declarations for test setup (wrappers, props, options)',
      recommended: true,
      url: 'https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/eslint-local-rules/no-module-scope-test-setup.md',
    },
    messages: {
      noModuleScopeWrapper:
        'buildWrapper() must not be called at module scope. Move it inside each test so every test is self-contained.',

      noModuleScopeWrapperHelper:
        "'{{ name }}' wraps buildWrapper() at module scope. Move the buildWrapper() call inline into each test so every test is self-contained.",

      noRenderWithinRenderHelper:
        "'{{ name }}' wraps render() in a local helper — call render() and buildWrapper() directly in each test instead.",

      noModuleScopeSetupConst:
        "'{{ name }}' is declared at module scope but looks like test setup (props, options, config). Inline it inside each test, or group shared setup in a nested describe with a factory function.",
    },
    schema: [],
  },

  create(context) {
    const TEST_HOOKS = new Set(['it', 'test', 'beforeEach', 'afterEach', 'beforeAll', 'afterAll']);

    function getCalleeName(callExpr) {
      const { callee } = callExpr;
      if (callee.type === 'Identifier') return callee.name;
      // describe.each, describe.skip, etc. — the name is on the object
      if (callee.type === 'MemberExpression' && callee.object?.type === 'Identifier') {
        return callee.object.name;
      }
      return null;
    }

    function hasParentDescribe(callExpr) {
      let current = callExpr.parent;
      while (current) {
        if (current.type === 'Program') return false;
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name === 'describe') return true;
        }
        current = current.parent;
      }
      return false;
    }

    function isStandaloneFunction(node) {
      return node.parent?.type !== 'CallExpression';
    }

    /**
     * The outermost describe() in a file is just a wrapper — variables there are shared across all
     * tests. Nested describes are semantic groups where shared state is the intended pattern.
     *
     * Walk up the AST:
     * - test hook (it/test/beforeEach/…) → inside a test → false
     * - nested describe (has a parent describe) → semantic group → false
     * - top-level describe (no parent describe) → file wrapper → true
     * - Program → module scope → true
     */
    function isModuleScope(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return true;
        if (
          (current.type === 'FunctionDeclaration' ||
            current.type === 'FunctionExpression' ||
            current.type === 'ArrowFunctionExpression') &&
          // Only standalone functions create a real scope boundary. Test callbacks
          // (arrows passed to it/describe/etc.) are transparent to this rule.
          isStandaloneFunction(current)
        ) {
          return false;
        }
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name && TEST_HOOKS.has(name)) return false;
          // Top-level describe is a file wrapper → module scope. Nested describe is a
          // semantic group → not module scope.
          if (name === 'describe') return !hasParentDescribe(current);
        }
        current = current.parent;
      }
      return true;
    }

    /**
     * Like isModuleScope, but a nested describe never exempts a function here: wrapping
     * render() in another helper is unnecessary indirection at any nesting depth, unlike
     * shared literal setup, where a nested describe is an intentional semantic group.
     */
    function isDescribeOrModuleScope(node) {
      let current = node.parent;
      while (current) {
        if (current.type === 'Program') return true;
        if (
          (current.type === 'FunctionDeclaration' ||
            current.type === 'FunctionExpression' ||
            current.type === 'ArrowFunctionExpression') &&
          isStandaloneFunction(current)
        ) {
          return false;
        }
        if (current.type === 'CallExpression') {
          const name = getCalleeName(current);
          if (name && TEST_HOOKS.has(name)) return false;
          if (name === 'describe') return true;
        }
        current = current.parent;
      }
      return true;
    }

    function isFunctionBoundary(node) {
      return (
        node.type === 'FunctionDeclaration' ||
        node.type === 'FunctionExpression' ||
        node.type === 'ArrowFunctionExpression'
      );
    }

    /**
     * Walks a statement/expression tree looking for a direct call to `targetName`
     * (e.g. `render`, `buildWrapper`), stopping at any nested function boundary — a call
     * buried inside an inner closure isn't necessarily executed when the outer helper
     * runs, so it doesn't count as the helper itself performing that call.
     *
     * Covers the shapes a call can hide behind: control flow (if/try/switch/loops),
     * boolean/ternary short-circuiting, intermediate variables, and nested call
     * arguments/array/object literals — not just a bare top-level statement.
     */
    function bodyCallsFunction(node, targetName) {
      if (!node) return false;

      if (node.type === 'CallExpression') {
        const { callee } = node;
        if (callee.type === 'Identifier' && callee.name === targetName) return true;
        return node.arguments.some((arg) => bodyCallsFunction(arg, targetName));
      }

      if (isFunctionBoundary(node)) return false;

      switch (node.type) {
        case 'Program':
        case 'BlockStatement':
          return node.body.some((n) => bodyCallsFunction(n, targetName));
        case 'ExpressionStatement':
          return bodyCallsFunction(node.expression, targetName);
        case 'ReturnStatement':
        case 'AwaitExpression':
        case 'UnaryExpression':
        case 'SpreadElement':
        case 'YieldExpression':
          return bodyCallsFunction(node.argument, targetName);
        case 'IfStatement':
          return (
            bodyCallsFunction(node.consequent, targetName) ||
            bodyCallsFunction(node.alternate, targetName)
          );
        case 'ConditionalExpression':
          return (
            bodyCallsFunction(node.test, targetName) ||
            bodyCallsFunction(node.consequent, targetName) ||
            bodyCallsFunction(node.alternate, targetName)
          );
        case 'LogicalExpression':
        case 'BinaryExpression':
        case 'AssignmentExpression':
          return (
            bodyCallsFunction(node.left, targetName) || bodyCallsFunction(node.right, targetName)
          );
        case 'SequenceExpression':
          return node.expressions.some((n) => bodyCallsFunction(n, targetName));
        case 'TryStatement':
          return (
            bodyCallsFunction(node.block, targetName) ||
            (node.handler ? bodyCallsFunction(node.handler.body, targetName) : false) ||
            bodyCallsFunction(node.finalizer, targetName)
          );
        case 'SwitchStatement':
          return node.cases.some((c) => c.consequent.some((n) => bodyCallsFunction(n, targetName)));
        case 'ForStatement':
        case 'ForInStatement':
        case 'ForOfStatement':
        case 'WhileStatement':
        case 'DoWhileStatement':
        case 'LabeledStatement':
          return bodyCallsFunction(node.body, targetName);
        case 'VariableDeclaration':
          return node.declarations.some((d) => bodyCallsFunction(d.init, targetName));
        case 'ArrayExpression':
          return node.elements.some((el) => bodyCallsFunction(el, targetName));
        case 'ObjectExpression':
          return node.properties.some((p) => bodyCallsFunction(p.value ?? p.argument, targetName));
        case 'TemplateLiteral':
          return node.expressions.some((n) => bodyCallsFunction(n, targetName));
        default:
          return false;
      }
    }

    /**
     * Heuristic: does this initializer look like test setup data?
     * Matches object literals, array literals, and JSX — the common shapes
     * for props / options / config objects.
     */
    function looksLikeSetupData(init) {
      if (!init) return false;
      return (
        init.type === 'ObjectExpression' ||
        init.type === 'ArrayExpression' ||
        init.type === 'JSXElement' ||
        init.type === 'JSXFragment'
      );
    }

    return {
      FunctionDeclaration(node) {
        const name = node.id?.name ?? '<anonymous>';

        if (isModuleScope(node) && bodyCallsFunction(node.body, 'buildWrapper')) {
          context.report({
            node,
            messageId: 'noModuleScopeWrapperHelper',
            data: { name },
          });
          return;
        }

        if (isDescribeOrModuleScope(node) && bodyCallsFunction(node.body, 'render')) {
          context.report({
            node,
            messageId: 'noRenderWithinRenderHelper',
            data: { name },
          });
        }
      },

      VariableDeclaration(node) {
        const declareScope = isModuleScope(node);
        const renderScope = declareScope || isDescribeOrModuleScope(node);
        if (!renderScope) return;

        for (const declarator of node.declarations) {
          const { init, id } = declarator;
          const name = id.type === 'Identifier' ? id.name : '<destructured>';
          const isHelperFunction =
            init && (init.type === 'ArrowFunctionExpression' || init.type === 'FunctionExpression');

          if (declareScope && bodyCallsFunction(init, 'buildWrapper')) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeWrapper',
            });
            continue;
          }

          if (declareScope && isHelperFunction && bodyCallsFunction(init.body, 'buildWrapper')) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeWrapperHelper',
              data: { name },
            });
            continue;
          }

          if (isHelperFunction && bodyCallsFunction(init.body, 'render')) {
            context.report({
              node: declarator,
              messageId: 'noRenderWithinRenderHelper',
              data: { name },
            });
            continue;
          }

          if (declareScope && looksLikeSetupData(init)) {
            context.report({
              node: declarator,
              messageId: 'noModuleScopeSetupConst',
              data: { name },
            });
          }
        }
      },
    };
  },
};

export default rule;
