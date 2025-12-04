---
name: Airbnb React/JSX Style Guide
description: "Apply React/JSX best practices and conventions from Airbnb's React Style Guide. Use when writing, reviewing, or refactoring React components to ensure clean, maintainable, and accessible implementations."
---

# Airbnb React/JSX Style Guide

Apply best practices and conventions from the [Airbnb React/JSX Style Guide](https://github.com/airbnb/javascript/tree/master/react) to write clean, idiomatic React code.

## When to Apply

Use this skill automatically when:
- Writing new React components
- Reviewing React/JSX code
- Refactoring existing React implementations

## Key Reminders

Follow the conventions documented at https://github.com/airbnb/javascript/tree/master/react, with particular attention to:

---

## Basic Rules

### Component Files

- **One component per file**: Each file should contain only one React component
- **Multiple stateless components allowed**: Exception for small, related stateless/pure components
- **Always use JSX syntax**: Don't use `React.createElement` unless initializing from non-JSX files
- **File extension**: Use `.jsx` for React component files

### Component Type Selection

- **Class components**: Use `class extends React.Component` when you have:
  - Internal state
  - Refs
  - Lifecycle methods
- **Function components**: Use regular functions for stateless components
- **Avoid arrow functions**: For stateless components, prefer regular functions for better function name inference

---

## Naming Conventions

### Files and Components

- **Filenames**: Use PascalCase (e.g., `ReservationCard.jsx`, `UserProfile.jsx`)
- **Component references**: PascalCase for components, camelCase for instances

```jsx
// ✅ Good
import ReservationCard from './ReservationCard';

const reservationItem = <ReservationCard />;

// ❌ Bad
import reservationCard from './ReservationCard';

const ReservationItem = <ReservationCard />;
```

### Props Naming

- **Use camelCase** for prop names
- **Use PascalCase** if prop value is a React component
- **Avoid DOM prop names** for different purposes (don't repurpose `style`, `className`, etc.)

```jsx
// ✅ Good
<MyComponent userName="hello" phoneNumber={12345678} Component={SomeComponent} />

// ❌ Bad
<MyComponent UserName="hello" phone_number={12345678} component={SomeComponent} />
```

### Higher-Order Components

- **Set displayName**: Use composite format `withFoo(ComponentName)`

```jsx
// ✅ Good
export default function withFoo(WrappedComponent) {
  function WithFoo(props) {
    return <WrappedComponent {...props} foo />;
  }

  const wrappedComponentName = WrappedComponent.displayName
    || WrappedComponent.name
    || 'Component';

  WithFoo.displayName = `withFoo(${wrappedComponentName})`;
  return WithFoo;
}
```

---

## Declaration

### Component Naming

- **Don't use displayName** for naming components
- **Name by reference** instead

```jsx
// ❌ Bad
export default React.createClass({
  displayName: 'ReservationCard',
  // stuff goes here
});

// ✅ Good
export default class ReservationCard extends React.Component {
}
```

---

## Formatting & Alignment

### JSX Alignment

**Multi-line JSX**: Break onto multiple lines with proper indentation

```jsx
// ✅ Good
<Foo
  superLongParam="bar"
  anotherSuperLongParam="baz"
>
  <Quux />
</Foo>

// ✅ Good - single line is fine when props fit
<Foo bar="bar" />

// ✅ Good - children on same line
<Foo
  superLongParam="bar"
  anotherSuperLongParam="baz"
/>

// ❌ Bad
<Foo superLongParam="bar"
     anotherSuperLongParam="baz" />
```

### Conditional Rendering

**Wrap with parentheses** for complex multi-line JSX

```jsx
// ✅ Good
{showButton && (
  <Button>
    Click Me
  </Button>
)}

// ✅ Good - simple single line
{showButton && <Button />}

// ❌ Bad
{showButton &&
  <Button>
    Click Me
  </Button>
}
```

---

## Quotes

### Quote Style

- **JSX attributes**: Always use double quotes (`"`)
- **All other JS**: Use single quotes (`'`)

```jsx
// ✅ Good
<Foo bar="bar" />
<Foo style={{ left: '20px' }} />

// ❌ Bad
<Foo bar='bar' />
<Foo style={{ left: "20px" }} />
```

---

## Spacing

### Self-Closing Tags

**Always include a single space** before self-closing

```jsx
// ✅ Good
<Foo />

// ❌ Bad
<Foo/>
<Foo                 />
```

### Curly Braces

**No padding inside** JSX curly braces

```jsx
// ✅ Good
<Foo bar={baz} />

// ❌ Bad
<Foo bar={ baz } />
```

---

## Props

### Boolean Props

**Omit the value** when explicitly `true`

```jsx
// ✅ Good
<Foo hidden />

// ❌ Bad
<Foo hidden={true} />
```

### Images

**Always include alt prop** on `<img>` tags

- Use empty string for presentational images
- **Avoid redundant words** like "image", "photo", "picture"

```jsx
// ✅ Good
<img src="hello.jpg" alt="Me waving hello" />
<img src="logo.jpg" alt="" /> {/* Presentational */}

// ❌ Bad
<img src="hello.jpg" />
<img src="hello.jpg" alt="Picture of me waving hello" />
```

### ARIA Roles

**Use only valid, non-abstract** ARIA roles

```jsx
// ✅ Good - valid ARIA role
<div role="button" />

// ❌ Bad - not an ARIA role
<div role="datepicker" />

// ❌ Bad - abstract ARIA role
<div role="range" />
```

### AccessKey

**Never use `accessKey`** attribute (inconsistent keyboard shortcuts)

```jsx
// ❌ Bad
<div accessKey="h" />

// ✅ Good
<div />
```

### Keys

**Avoid array indexes as keys** - use stable IDs instead

```jsx
// ❌ Bad
{todos.map((todo, index) =>
  <Todo
    {...todo}
    key={index}
  />
)}

// ✅ Good
{todos.map(todo => (
  <Todo
    {...todo}
    key={todo.id}
  />
))}
```

### Default Props

**Always define defaultProps** for all non-required props

```jsx
// ✅ Good
function SFC({ foo, bar, children }) {
  return <div>{foo}{bar}{children}</div>;
}
SFC.propTypes = {
  foo: PropTypes.number.isRequired,
  bar: PropTypes.string,
  children: PropTypes.node,
};
SFC.defaultProps = {
  bar: '',
  children: null,
};
```

### Props Spreading

**Use spread sparingly** and filter out unnecessary props

```jsx
// ✅ Good - specific props pulled out
function Input(props) {
  const { type, ...other } = props;
  return <input type={type} {...other} />;
}

// ❌ Bad - spreads all props indiscriminately
function Input(props) {
  return <input {...props} />;
}
```

### PropTypes Validation

**Use explicit PropTypes** for arrays and objects

```jsx
// ✅ Good
Component.propTypes = {
  items: PropTypes.arrayOf(PropTypes.string),
  user: PropTypes.shape({
    name: PropTypes.string,
    age: PropTypes.number,
  }),
};

// ❌ Bad
Component.propTypes = {
  items: PropTypes.array,
  user: PropTypes.object,
};
```

---

## Refs

### Ref Callbacks

**Always use ref callbacks** instead of string refs

```jsx
// ❌ Bad
<Foo ref="myRef" />

// ✅ Good
<Foo
  ref={(ref) => { this.myRef = ref; }}
/>
```

---

## Parentheses

### Multi-line JSX

**Wrap in parentheses** when JSX spans multiple lines

```jsx
// ✅ Good
render() {
  return (
    <MyComponent variant="long">
      <MyChild />
    </MyComponent>
  );
}

// ✅ Good - single line doesn't need parens
render() {
  return <MyComponent variant="short" />;
}

// ❌ Bad
render() {
  return <MyComponent variant="long">
           <MyChild />
         </MyComponent>;
}
```

---

## Tags

### Self-Closing

**Always self-close** tags that have no children

```jsx
// ✅ Good
<Foo variant="stuff" />

// ❌ Bad
<Foo variant="stuff"></Foo>
```

### Multi-line Components

**Closing bracket on new line** for multi-property components

```jsx
// ✅ Good
<Foo
  bar="bar"
  baz="baz"
/>

// ❌ Bad
<Foo
  bar="bar"
  baz="baz" />
```

---

## Methods

### Event Handlers

**Bind in constructor**, not in render

```jsx
// ❌ Bad - creates new function on every render
class extends React.Component {
  onClickDiv() {
    // do stuff
  }

  render() {
    return <div onClick={this.onClickDiv.bind(this)} />;
  }
}

// ✅ Good
class extends React.Component {
  constructor(props) {
    super(props);
    this.onClickDiv = this.onClickDiv.bind(this);
  }

  onClickDiv() {
    // do stuff
  }

  render() {
    return <div onClick={this.onClickDiv} />;
  }
}
```

### Arrow Functions in Render

**Avoid when possible**, but acceptable for passing extra parameters

```jsx
// ❌ Bad
class ItemList extends React.Component {
  render() {
    return (
      <ul>
        {this.props.items.map((item, index) => (
          <Item
            key={item.key}
            onClick={() => doSomethingWith(item.name, index)}
          />
        ))}
      </ul>
    );
  }
}

// ✅ Good - when you need to pass parameters
<Button onClick={(e) => this.handleClick(id, e)} />
```

### Internal Methods

**Don't prefix with underscores** for internal methods

```jsx
// ❌ Bad
React.createClass({
  _onClickSubmit() {
    // do stuff
  },
});

// ✅ Good
class extends React.Component {
  onClickSubmit() {
    // do stuff
  }
}
```

### Return in Render

**Always return a value** in `render` method

```jsx
// ❌ Bad
render() {
  (<div />);
}

// ✅ Good
render() {
  return (<div />);
}
```

---

## Ordering

### Class Component Method Order

1. **Static methods and properties**
2. `constructor`
3. `getChildContext`
4. `componentWillMount`
5. `componentDidMount`
6. `componentWillReceiveProps`
7. `shouldComponentUpdate`
8. `componentWillUpdate`
9. `componentDidUpdate`
10. `componentWillUnmount`
11. **Event handlers** (e.g., `onClickSubmit()`, `onChangeDescription()`)
12. **Getter methods for render** (e.g., `getSelectReason()`, `getFooterContent()`)
13. **Optional render methods** (e.g., `renderNavigation()`, `renderProfilePicture()`)
14. `render`

### Static Properties

Define after class definition

```jsx
class MyComponent extends React.Component {
  // class methods
}

MyComponent.propTypes = {
  // prop types
};

MyComponent.defaultProps = {
  // default props
};
```

---

## Anti-Patterns to Avoid

### Never Use

- ❌ **Mixins**: Create implicit dependencies and cause name clashes
- ❌ **isMounted**: Deprecated and indicates code smell (use proper lifecycle methods)
- ❌ **String refs**: Use callback refs instead
- ❌ **Array indices as keys**: Use stable unique IDs
- ❌ **accessKey attribute**: Causes accessibility issues
- ❌ **Generic DOM props for custom purposes**: Don't repurpose `style`, `className`, etc.

### Avoid When Possible

- ❌ Creating functions in render (use bind in constructor)
- ❌ Indiscriminate prop spreading (filter unnecessary props)
- ❌ Missing PropTypes validation
- ❌ Missing defaultProps for optional props
- ❌ Using `any`, `array`, `object` in PropTypes (be specific)

---

## Summary Checklist

When writing React components:

- [ ] One component per file with `.jsx` extension
- [ ] Used PascalCase for component files and references
- [ ] Props use camelCase (PascalCase for component values)
- [ ] Multi-line JSX wrapped in parentheses and properly aligned
- [ ] Double quotes for JSX attributes, single quotes for JS
- [ ] Single space before self-closing tags
- [ ] No spaces inside curly braces
- [ ] Omitted `true` value for boolean props
- [ ] Included `alt` on all images
- [ ] Used stable IDs for keys (not array indices)
- [ ] Defined defaultProps for all optional props
- [ ] Used callback refs instead of string refs
- [ ] Bound event handlers in constructor
- [ ] Defined explicit PropTypes (not generic array/object)
- [ ] Followed proper method ordering
- [ ] Always return value from render

## References

- Official Guide: https://github.com/airbnb/javascript/tree/master/react
- ESLint Config: https://www.npmjs.com/package/eslint-config-airbnb
- React PropTypes: https://reactjs.org/docs/typechecking-with-proptypes.html
