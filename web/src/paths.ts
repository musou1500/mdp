// URL paths are kept encoded while navigating and decoded when compared with
// the plain paths the server sends in the tree.

export function decodePath(pathname: string): string {
  try {
    return decodeURIComponent(pathname)
  } catch {
    return pathname
  }
}

// isViewerPath reports whether a path is handled by the viewer rather than by
// a plain browser request: a markdown file, or a directory that may have one.
export function isViewerPath(pathname: string): boolean {
  return /\.(md|markdown)$/i.test(pathname) || pathname.endsWith('/')
}
