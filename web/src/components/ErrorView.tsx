import { isRouteErrorResponse, useRouteError } from 'react-router'

export function ErrorView() {
  const error = useRouteError()
  const [heading, detail] = describe(error)
  return (
    <div className="error">
      <h1>{heading}</h1>
      <p>{detail}</p>
    </div>
  )
}

function describe(error: unknown): [string, string] {
  if (isRouteErrorResponse(error)) {
    return [`${error.status}`, error.statusText || error.data || 'Request failed']
  }
  if (error instanceof Error) {
    return ['Error', error.message]
  }
  return ['Error', String(error)]
}
