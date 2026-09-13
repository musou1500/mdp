# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 概要

`mdp [-addr host:port] [docroot]` は、マークダウンのディレクトリをライブプレビューとして配信する。
ドキュメントルートに対する普通の静的 web サーバとして振る舞い、例外は 2 つだけ。マークダウンファイル
(または `README.md`/`readme.md`/`index.md` を持つディレクトリ) を指す URL は生のバイト列ではなく React
ビューアを返し、`/_md/` はビューア自身のエンドポイント用に予約されている。

## コマンド

```sh
make build           # npm install && vite build のあと go build -o mdp .
make build-windows   # 同上、GOOS=windows GOARCH=amd64 で mdp.exe を生成
make web             # フロントエンドのみ -> internal/server/dist
make test            # go vet ./... && go test ./... && cd web && npx tsc --noEmit
make dev             # ビルドして ./testdata/docroot を 127.0.0.1:8080 で配信
```

Go のテストを 1 つだけ実行する: `go test ./internal/render -run TestAbsolutize -v`。
フロントエンドにテストランナーはなく、`web/` での `npx tsc --noEmit` が唯一のチェック。

ビルド順序に意味がある。`internal/server` は `internal/server/dist` を embed するので、`go build` の前に
ビューアが存在していなければならない。ビルド成果物はコミットされているため、フロントエンドに変更がなければ
`go build` だけで足りる。`dist/index.html` が無い場合、`server.New` は
"viewer is missing, run `make web`" で失敗する。

## モジュールツリー

全体で 2000 行未満。依存の向きは `server` → `render`/`tree`/`watch` の一方向で、例外は 2 つ。
`watch` が `tree.SkipDir` を借り、`tree` がサイドバー用のタイトルに `render.Title` を借りている。

```
main.go                起動: フラグ解析、docroot の検証、リッスン、シグナルでの停止
internal/render        マークダウン -> HTML フラグメント。goldmark + chroma、タイトル抽出
internal/tree          サイドバー用ツリー (名前とタイトル) の構築と、無視するファイル/拡張子の判定
internal/watch         fsnotify の再帰監視。変更があったことだけを合図する (中身は運ばない)
internal/server        ルーティング、静的配信、/_md/ の各エンドポイント、SSE のファンアウト
internal/server/dist   生成物 (Vite の出力)。embed 対象なので手で編集しない
web/src                React ビューア: ルーティング、/_md/ クライアント、SSE 購読、パス変換
web/src/components     画面: レイアウト、サイドバー、ドキュメント、目次、エラー表示
testdata/docroot       テストと `make dev` が使うサンプルドキュメントルート
```

## アーキテクチャ

リクエストの流れ: `main.go` が docroot を絶対パスに解決して `server.New(ctx, root)` に渡す。これは
`http.Handler` を返し、同時にファイル監視を開始する (監視は `ctx` のキャンセルで停止する)。

`internal/server` のルート:

| ルート | 振る舞い |
|-------|----------|
| `/_md/tree` | JSON `[]tree.Node` — マークダウンのファイルツリー (名前 + タイトル)。リクエストごとに再構築 |
| `/_md/doc?path=` | JSON `{path,title,html,headings}` — レンダリング済みのドキュメント 1 件 |
| `/_md/events` | 変更の合図の SSE ストリーム (`data: change`)。30 秒ごとにコメント行でハートビート |
| `/_md/highlight.css` | chroma のスタイルシート。`sync.OnceValue` で 1 度だけ生成 |
| `/_md/assets/` | embed した Vite の出力。`Cache-Control: immutable` |
| `/` (それ以外すべて) | マークダウンのパスとインデックスを持つディレクトリはビューアの shell、それ以外は静的ファイル |

ライブリロード: fsnotify (再帰的に監視し、新規ディレクトリには監視を追加し直す) → `watch` が 100ms 静かに
なるまで待って 1 回合図する → `hub` が購読者に配る → SSE → `useEvents` が 50ms デバウンス → React Router の
`revalidate()`。revalidate はツリーとドキュメント両方のローダーを再実行するが再マウントはしないので、
スクロール位置が保たれる。ここを再マウントや手動リロードに置き換えないこと。

**この経路は「何が変わったか」を一切運ばない。** `watch.Watch` が返すのは `<-chan struct{}` で、SSE の
payload は定数 `data: change`、`useEvents` はそれをパースしない。現在の状態を答えられるのはディスクだけ
なので、判断は `/_md/tree` と `/_md/doc` の再取得に一任している。`watch` の出力チャネルは容量 1 で、
未読の合図があれば新しい合図は捨てる — 同じ意味だから。

`hub` は購読者のマップを持たない。Go にあるブロードキャストはチャネルの close だけで、それは 1 度しか
撃てないので、`broadcast` は現在のチャネルを閉じて新しいものに差し替える。**`hub.wait()` は毎周回読み直す
こと** — 変更を報告したチャネルは使用済みで、握り続けると 2 回目以降の変更を取りこぼす。購読者ごとに
違うデータを配る必要が出たらこの方式は成立しないので、そのときはマップに戻す。

`New` に渡した ctx のキャンセルで監視が止まると、`server.done` 経由で開いている SSE ストリームも終わる。
これは飾りではなく、`main.go` の `srv.Shutdown` が進行中のリクエストを待つので、これが無いと停止しない。

サイドバーの楽観的更新のように差分が必要になったら、`watch` にイベント型を戻すのではなく、`server` が
直前の `tree.Build` の結果を保持して差分を計算する層を足すこと。ツリーの所属は非局所的な派生プロパティ
(マークダウンを含まないディレクトリは除外されるので、深い `.md` 1 つの増減で祖先が連鎖する) なので、
単一のファイルシステムイベントからは導けない。また保存はツリーを変えずに本文だけ変えるため、
「ツリーの差分」だけでは足りず本文用の合図が別に必要になる。

### Go と TS をまたぐ規約

- **パスはどこでも「エスケープしていない URL パス」**: ファイルは `/docs/a.md`、ディレクトリは末尾スラッシュ
  付きの `/docs/`。`tree.Node` と `doc.Path` のどちらも同じ形。URL パスを `fs.FS`/`os.Root`
  が期待する相対形に変換するのは `server.relName` ただ 1 箇所 (`/` → `.`)。フロントエンドはナビゲーション中は
  パスをエンコードしたまま保持し (サイドバーの `encodeURI`)、サーバから来たパスと比較する前にデコードする
  — [web/src/paths.ts](web/src/paths.ts) を参照。
- **ディレクトリトラバーサル対策は `os.Root`** によるもので、パス文字列のチェックではない。ドキュメントルート
  以下を読むのは静的配信も `tree.Build` もすべて `osRoot.FS()` 経由で、`server` がディスクに触る窓口は
  `s.root` 1 つに集約されている (fsnotify が実パスを要求する `internal/watch` だけが例外)。
- **`tree.SkipDir`** (ドットファイル、`node_modules`) はツリー構築と監視の両方から使われる。一覧に出るものと
  監視されるものを一致させるため。
- **`render.Render` が返すのはページではなく HTML フラグメント**。相対リンク・画像の宛先はドキュメントの
  ディレクトリを基準に絶対 URL パスへ書き換えられ、フラグメントがどこに挿入されても正しくなる。
- **見出しの一覧 (`Document.Headings`) も `Render` が本文と一緒に返す**。目次は本文中の `id` へ飛ぶので、
  一覧と HTML の `id` が同じ 1 回の走査から出ていないと壊れる。フロントエンドは `<a href="#id">` を並べる
  だけで、スクロールはブラウザに任せる。見出しが 1 つ (= 表題のみ) のときは目次を出さない。
- **見出しやタイトルの文字列は「読み手が見る文字」**。goldmark の AST はソースのまま (`100\% done`,
  `AT&amp;T`) を保持していて、解釈するのはレンダラなので、`nodeText` が `plainText` で同じ変換
  (バックスラッシュエスケープと文字参照) を掛けている。ここを外すと本文とタブ名・サイドバー・目次で
  表示が食い違う。コードスパンなど raw のノードは本文でも解釈されないのでそのまま。
- **タイトルは「最初のレベル 1 見出し」で、判定は `render` に 1 つ**。本文は `Render` が返し、サイドバーは
  `render.Title` を使う (パースのみで HTML は作らない) ので、タブ名と一覧が食い違わない。`tree.Build` は
  そのためにマークダウンファイルを全部読む。無題のときサーバは `doc.Title` をファイル名で埋めるが、
  `tree.Node.Title` は空のままにする — サイドバーはファイル名を別途表示しているので、同じ文字列を
  2 度並べないため。
- Vite の `base` は `/_md/`、`outDir` は `../internal/server/dist`。どちらを変えるにも `server.New` の
  ルート定義側の対応する変更が要る。

### 意図的な選択 (壊さないこと)

- マークダウン中の生 HTML を許可し (`html.WithUnsafe()`)、フロントエンドは `dangerouslySetInnerHTML` を
  使う。対象はユーザ自身のディスク上のファイルであり、信頼できない入力ではないため。
- 見出しの `id` は goldmark の自動生成ではなく `render.identifyHeadings` が振る。自動生成は ASCII 以外を
  落として日本語見出しをすべて `heading`, `heading-1`, … に潰すため、こちらは字種を問わず文字・数字を残し、
  重複した見出しにだけ `-1`, `-2`, … を足す。
- ハイライト用 CSS は chroma の各テーマをそれぞれの `prefers-color-scheme` メディアクエリで囲む。2 つの
  テーマはセレクタを共有するので、囲まないとライトの色がダークモードに漏れる。

`testdata/docroot` には非 ASCII のファイル名、ネストしたガイド、画像、ルートとネストの両方の `README.md` を
意図的に置いてある。フィクスチャを追加するときもこの性質を保つこと。
