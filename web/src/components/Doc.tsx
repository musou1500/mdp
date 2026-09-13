import { useEffect } from 'react'
import type { MouseEvent } from 'react'
import { useLoaderData, useLocation, useNavigate } from 'react-router'
import type { Doc as DocData } from '../api'
import { isViewerPath } from '../paths'
import { Toc } from './Toc'

export function Doc() {
  const doc = useLoaderData() as DocData
  const navigate = useNavigate()
  const { hash } = useLocation()

  useEffect(() => {
    document.title = doc.title
  }, [doc.title])

  useEffect(() => {
    if (!hash) return
    document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView()
  }, [doc.path, hash])

  // Links inside the rendered markdown are plain <a> elements, so navigation
  // between documents is intercepted here. Everything else - images, external
  // links, in-page anchors - is left to the browser.
  function onClick(e: MouseEvent<HTMLElement>) {
    if (e.defaultPrevented || e.button !== 0) return
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor || anchor.target || anchor.hasAttribute('download')) return

    const url = new URL(anchor.href)
    if (url.origin !== window.location.origin) return
    if (url.pathname === window.location.pathname) return // in-page anchor
    if (!isViewerPath(url.pathname)) return

    e.preventDefault()
    navigate(url.pathname + url.search + url.hash)
  }

  return (
    <div className="doc">
      <article
        className="markdown-body"
        onClick={onClick}
        dangerouslySetInnerHTML={{ __html: doc.html }}
      />
      {/* A single heading is the document's own title; it is no table of contents. */}
      {doc.headings.length > 1 && <Toc headings={doc.headings} />}
    </div>
  )
}
