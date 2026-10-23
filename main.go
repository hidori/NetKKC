package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hidori/NetKKC/internal/httpapi"
	"github.com/hidori/NetKKC/internal/mozc"
)

func main() {
	profile := flag.String("mozc-profile", defaultProfile(), "dedicated Mozc profile directory (not a desktop IME profile)")
	address := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()
	client := &mozc.Client{Profile: *profile}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := client.Configure(ctx); err != nil {
		log.Printf("Mozc initialization failed; conversions will return an error until it is available: %v", err)
	}
	cancel()
	e := httpapi.New(mozc.NewConverter(client))
	e.Logger.Fatal(e.Start(*address))
}

func defaultProfile() string {
	if profile := os.Getenv("MOZC_PROFILE_DIR"); profile != "" {
		return profile
	}
	if _, err := os.Stat("/workspace/.netkkc/home/.config/mozc"); err == nil {
		return "/workspace/.netkkc/home/.config/mozc"
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "netkkc", "mozc")
}
