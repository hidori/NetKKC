package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hidori/NetKKC/internal/httpapi"
	"github.com/hidori/NetKKC/internal/mozc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	profile := flag.String("mozc-profile", os.Getenv("MOZC_PROFILE_DIR"), "dedicated Mozc profile directory (not a desktop IME profile)")
	address := flag.String("listen", ":8080", "HTTP listen address")

	flag.Parse()

	client := &mozc.Client{Profile: *profile}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := waitForMozc(ctx, client)

	cancel()

	if err != nil {
		return err
	}

	e := httpapi.New(mozc.NewConverter(client))
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.IdleTimeout = time.Minute

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()

		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = e.Shutdown(shutdown)
	}()

	if err := e.Start(*address); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

func waitForMozc(ctx context.Context, client *mozc.Client) error {
	for {
		err := client.Configure(ctx)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
}
