# mdp

A markdown preview server for a whole directory.

```sh
mdp ./docroot
```

It behaves like an ordinary static web server for the directory you point it
at, so `![](./img/logo.png)` and `[guide](../guide/install.md)` resolve exactly
as they would on a real site. The only exceptions:

- a URL that names a markdown file (or a directory holding `README.md` or
  `index.md`) opens the viewer instead of the raw file;
- `/_md/` is reserved for the viewer's own endpoints.

The viewer has a sidebar with every markdown file under the document root, and
follows the files as you edit them: saving, adding or deleting a file updates
the page over an event stream, with no manual reload and without losing your
scroll position.

## Options

    -addr host:port   address to listen on (default 127.0.0.1:8080)

The document root defaults to the current directory. Add `?raw` to a markdown
URL to read its source.

## Building

The viewer is a React app that is compiled into the binary, so building it
needs Node once:

```sh
make build           # npm install && vite build, then go build -> ./mdp
make build-windows   # same, cross-compiled to ./mdp.exe
make test            # go vet, go test, tsc --noEmit
make dev             # build and serve ./testdata/docroot
```

The build output is committed under `internal/server/dist`, so `go build`
alone is enough when the frontend has not changed, and the resulting binary
runs anywhere on its own.

## Layout

| path | what it does |
|------|--------------|
| `internal/render` | markdown to HTML (goldmark + chroma), relative links made absolute |
| `internal/tree` | the markdown file tree under the document root |
| `internal/watch` | recursive file watching (fsnotify), debounced |
| `internal/server` | static files, the JSON endpoints, the event stream |
| `web` | the React viewer (Vite, React Router) |
