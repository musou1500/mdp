import { useCallback } from 'react'
import { Outlet, useLoaderData, useRevalidator } from 'react-router'
import type { TreeNode } from '../api'
import { useEvents } from '../useEvents'
import { Sidebar } from './Sidebar'

export function Layout() {
  const tree = useLoaderData() as TreeNode[]
  const { revalidate } = useRevalidator()

  // One revalidation re-runs both loaders, so the tree and the document are
  // refreshed together. Nothing is remounted, so the scroll position stays.
  useEvents(
    useCallback(() => {
      revalidate()
    }, [revalidate]),
  )

  return (
    <div className="app">
      <Sidebar nodes={tree} />
      <main className="content">
        <Outlet />
      </main>
    </div>
  )
}
