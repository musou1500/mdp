import { Link, useLocation, useMatches } from 'react-router'
import type { Doc, TreeNode } from '../api'
import { decodePath } from '../paths'

export function Sidebar({ nodes }: { nodes: TreeNode[] }) {
  const current = useCurrentPath()
  return (
    <nav className="sidebar">
      <div className="sidebar-title">mdp</div>
      {nodes.length === 0 ? (
        <p className="sidebar-empty">No markdown files.</p>
      ) : (
        <NodeList nodes={nodes} current={current} />
      )}
    </nav>
  )
}

// useCurrentPath returns the path of the document on screen. It is taken from
// the loaded document rather than the URL, because a directory URL such as
// /docs/ shows the file /docs/README.md.
function useCurrentPath(): string {
  const matches = useMatches()
  const doc = matches[matches.length - 1]?.loaderData as Doc | undefined
  const pathname = decodePath(useLocation().pathname)
  return doc?.path ?? pathname
}

function NodeList({ nodes, current }: { nodes: TreeNode[]; current: string }) {
  return (
    <ul className="tree">
      {nodes.map((node) => (
        <li key={node.path}>
          {node.isDir ? (
            // <details> gives collapsing without any state of our own.
            <details open>
              <summary>{node.name}</summary>
              <NodeList nodes={node.children ?? []} current={current} />
            </details>
          ) : (
            <Link
              to={encodeURI(node.path)}
              className={node.path === current ? 'file current' : 'file'}
            >
              <span className="file-name">{node.name}</span>
              {node.title && node.title !== node.name && (
                <span className="file-title">{node.title}</span>
              )}
            </Link>
          )}
        </li>
      ))}
    </ul>
  )
}
