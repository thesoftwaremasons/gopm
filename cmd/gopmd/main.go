// Command gopmd is the long-running process supervisor daemon for gopm.
// The CLI talks to it over a loopback TCP socket; users normally do not
// run it directly — the gopm CLI auto-launches it on first use.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/thesoftwaremasons/gopm/internal/daemon"
	"github.com/thesoftwaremasons/gopm/internal/ipc"
	"github.com/thesoftwaremasons/gopm/internal/state"
	"github.com/thesoftwaremasons/gopm/internal/stats"
	"github.com/thesoftwaremasons/gopm/internal/supervisor"
)

func main() {
	log.SetPrefix("gopmd: ")
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// When launched in the background by the CLI we have no console;
	// redirect log output to a file so startup errors are recoverable.
	if daemon.IsDaemonChild() {
		if home, err := os.UserHomeDir(); err == nil {
			logDir := filepath.Join(home, ".gopm")
			if err := os.MkdirAll(logDir, 0o755); err == nil {
				logPath := filepath.Join(logDir, "gopmd.log")
				if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
					log.SetOutput(f)
				}
			}
		}
	}

	statePath, err := state.DefaultPath()
	if err != nil {
		log.Printf("warning: state disabled: %v", err)
	}
	store := state.New(statePath)

	sup := supervisor.New(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Feature 5: start CPU sampler.
	stats.StartCPUSampler(ctx)

	handler := sup.Handler()
	srv := ipc.NewServer(ipc.DefaultAddr, func(ctx context.Context, req *ipc.Request) *ipc.Response {
		if req.Command == ipc.CmdShutdown {
			go cancel()
			return &ipc.Response{OK: true, Message: "daemon shutting down"}
		}
		return handler(ctx, req)
	})

	// Feature 3: set stream handler for log streaming and exec.
	srv.SetStreamHandler(sup.StreamHandler())

	if err := srv.Listen(); err != nil {
		fmt.Fprintln(os.Stderr, "gopmd:", err)
		os.Exit(1)
	}
	log.Printf("listening on %s", ipc.DefaultAddr)

	// Restore previously-running apps, if any.
	if st, err := store.Load(); err != nil {
		log.Printf("warning: load state: %v", err)
	} else if len(st.Apps) > 0 {
		log.Printf("restoring %d app(s) from %s", len(st.Apps), statePath)
		for _, a := range st.Apps {
			if err := sup.StartApp(a.Config); err != nil {
				log.Printf("restore %s: %v", a.Config.Name, err)
			}
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals()...)
	go func() {
		s := <-sigCh
		log.Printf("received %s, shutting down", s)
		cancel()
	}()

	go func() {
		if err := srv.Serve(ctx); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	sup.Shutdown()
	_ = srv.Close()
	log.Printf("shutdown complete")
}
