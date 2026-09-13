import type { Heading } from '../api'

// Toc lists the document's headings beside it. The links are plain anchors to
// ids inside the rendered fragment, so the browser does the jump itself.
export function Toc({ headings }: { headings: Heading[] }) {
  const top = Math.min(...headings.map((h) => h.level))
  return (
    <nav className="toc" aria-label="Table of contents">
      <div className="toc-title">On this page</div>
      <ul>
        {headings.map((h) => (
          <li key={h.id} style={{ paddingLeft: `${(h.level - top) * 0.75}rem` }}>
            <a href={`#${encodeURIComponent(h.id)}`}>{h.text}</a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
