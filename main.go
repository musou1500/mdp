// Command mdp serves a directory of markdown files as a live preview.
//
//	mdp [-addr host:port] [docroot]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"mdp/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mdp:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: mdp [-addr host:port] [docroot]")
		flag.PrintDefaults()
	}
	flag.Parse()

	docroot := "."
	switch flag.NArg() {
	case 0:
	case 1:
		docroot = flag.Arg(0)
	default:
		flag.Usage()
		return errors.New("too many arguments")
	}
	root, err := filepath.Abs(docroot)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(root); err != nil {
		return err
	} else if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	handler, err := server.New(ctx, root)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Printf("mdp serving %s on http://%s/\n", root, ln.Addr())

	srv := &http.Server{Handler: handler}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
