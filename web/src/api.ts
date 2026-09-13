// The endpoints the Go server exposes under /_md/. Paths in these types are
// URL paths of the document root, unescaped: "/docs/a.md", "/docs/".

export type TreeNode = {
  name: string
  // The document's own title. Absent for directories and for files without one.
  title?: string
  path: string
  isDir: boolean
  children?: TreeNode[]
}

export type Doc = {
  path: string
  title: string
  html: string
  headings: Heading[]
}

// A heading of the document, in document order. id is the id the heading
// carries in html, so the table of contents links to it.
export type Heading = {
  level: number
  text: string
  id: string
}

export type MdEvent = {
  type: 'create' | 'write' | 'remove'
  path: string
  isDir: boolean
}

export async function fetchTree(signal?: AbortSignal): Promise<TreeNode[]> {
  const res = await fetch('/_md/tree', { signal })
  if (!res.ok) throw await asResponse(res)
  return res.json()
}

export async function fetchDoc(path: string, signal?: AbortSignal): Promise<Doc> {
  const res = await fetch(`/_md/doc?path=${encodeURIComponent(path)}`, { signal })
  if (!res.ok) throw await asResponse(res)
  return res.json()
}

// asResponse turns a failed request into a Response that React Router renders
// through the route's ErrorBoundary.
async function asResponse(res: Response): Promise<Response> {
  let message = res.statusText
  try {
    const body = await res.json()
    if (typeof body?.error === 'string') message = body.error
  } catch {
    // Not a JSON error; the status text will do.
  }
  return new Response(message, { status: res.status, statusText: message })
}
