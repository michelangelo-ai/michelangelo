import { render } from '@testing-library/react';

import { Markdown } from '../markdown';

describe('Markdown component', () => {
  it('should render basic markdown correctly', () => {
    const { container } = render(<Markdown># Heading\n**Bold** and *italic* text</Markdown>);
    expect(container).toMatchInlineSnapshot(`
      <div>
        <h1
          id="headingnbold-and-italic-text"
        >
          Heading\\n
          <strong
            class=""
          >
            Bold
          </strong>
           and 
          <em>
            italic
          </em>
           text
        </h1>
      </div>
    `);
  });

  it('should render code blocks correctly', () => {
    const { container } = render(
      <Markdown>```javascript const hello = 'world'; console.log(hello); ```</Markdown>
    );
    expect(container).toMatchInlineSnapshot(`
      <div>
        <code>
          javascript const hello = 'world'; console.log(hello);
        </code>
      </div>
    `);
  });

  it('should render links correctly', () => {
    const { container } = render(<Markdown>[Link text](https://example.com)</Markdown>);
    expect(container).toMatchInlineSnapshot(`
      <div>
        <a
          class=""
          data-baseweb="link"
          href="https://example.com"
          rel="noopener noreferrer"
          target="_blank"
        >
          Link text
        </a>
      </div>
    `);
  });

  it('should render lists correctly', () => {
    const { container } = render(<Markdown>- Item 1 - Item 2 - Item 3</Markdown>);
    expect(container).toMatchInlineSnapshot(`
      <div>
        <ul
          class=""
        >
          <li
            class=""
          >
            Item 1 - Item 2 - Item 3
          </li>
        </ul>
      </div>
    `);
  });
});
