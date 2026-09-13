import type { RouteObject } from 'react-router'
import { fetchDoc, fetchTree } from './api'
import { Doc } from './components/Doc'
import { ErrorView } from './components/ErrorView'
import { Layout } from './components/Layout'

// The server only serves the viewer for markdown paths, so every route below
// is a document. Splat params arrive already decoded.
export const routes: RouteObject[] = [
  {
    id: 'root',
    path: '/',
    Component: Layout,
    loader: ({ request }) => fetchTree(request.signal),
    ErrorBoundary: ErrorView,
    children: [
      {
        index: true,
        Component: Doc,
        loader: ({ request }) => fetchDoc('/', request.signal),
        ErrorBoundary: ErrorView,
      },
      {
        path: '*',
        Component: Doc,
        loader: ({ params, request }) => fetchDoc('/' + (params['*'] ?? ''), request.signal),
        ErrorBoundary: ErrorView,
      },
    ],
  },
]
