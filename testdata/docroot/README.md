# mdp sample

This directory exists to exercise the viewer by hand. It is served as the
document root by `make dev`.

## Relative references

An image next to this file, resolved as a plain static request:

![logo](./img/logo.png)

A link to [another document](./docs/guide/install.md), and one to a
[directory](./docs/) that falls back to its `README.md`.

An [external link](https://example.com) should leave the viewer.

## GFM

| feature | supported |
|---------|-----------|
| tables  | yes       |
| task lists | yes    |

- [x] strikethrough: ~~gone~~
- [ ] autolinks: https://example.com

## Code

```go
func main() {
	http.Handle("/", http.FileServer(http.Dir(".")))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

```python
def greet(name: str) -> str:
    return f"hello, {name}"
```

> A blockquote, with `inline code` and **bold** text.
