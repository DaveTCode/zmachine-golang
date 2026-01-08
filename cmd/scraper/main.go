package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const url = "https://www.ifarchive.org/indexes/if-archive/games/zcode/"
const outputDir = "stories"

func main() {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Download the index page
	c := &http.Client{Timeout: 30 * time.Second}
	res, err := c.Get(url)
	if err != nil {
		fmt.Printf("Failed to fetch index: %v\n", err)
		os.Exit(1)
	}
	defer res.Body.Close() // nolint:errcheck

	if res.StatusCode != 200 {
		fmt.Printf("Bad status code: %d\n", res.StatusCode)
		os.Exit(1)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		fmt.Printf("Failed to parse HTML: %v\n", err)
		os.Exit(1)
	}

	// Find all Z-machine game links
	var games []struct {
		name string
		url  string
	}

	doc.Find("dl dt").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Find("a").Attr("href")
		if !exists {
			return
		}

		match, _ := regexp.Match(`.*\.z[12345678]$`, []byte(href))
		if match {
			name := filepath.Base(href)
			games = append(games, struct {
				name string
				url  string
			}{
				name: name,
				url:  "https://www.ifarchive.org" + href,
			})
		}
	})

	fmt.Printf("Found %d games to download\n", len(games))

	// Download each game
	downloaded := 0
	skipped := 0
	failed := 0

	for i, game := range games {
		destPath := filepath.Join(outputDir, game.name)

		// Skip if already exists
		if _, err := os.Stat(destPath); err == nil {
			fmt.Printf("[%d/%d] Skipping %s (already exists)\n", i+1, len(games), game.name)
			skipped++
			continue
		}

		fmt.Printf("[%d/%d] Downloading %s... ", i+1, len(games), game.name)

		resp, err := c.Get(game.url)
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
			continue
		}

		if resp.StatusCode != 200 {
			fmt.Printf("FAILED: status %d\n", resp.StatusCode)
			resp.Body.Close() // nolint:errcheck
			failed++
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close() // nolint:errcheck
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
			continue
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failed++
			continue
		}

		fmt.Printf("OK (%d bytes)\n", len(data))
		downloaded++

		// Be nice to the server
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\nDone! Downloaded: %d, Skipped: %d, Failed: %d\n", downloaded, skipped, failed)

	// Also create a manifest file
	manifestPath := filepath.Join(outputDir, "manifest.txt")
	var manifest strings.Builder
	for _, game := range games {
		manifest.WriteString(game.name + "\n")
	}
	os.WriteFile(manifestPath, []byte(manifest.String()), 0644) // nolint:errcheck
	fmt.Printf("Wrote manifest to %s\n", manifestPath)
}
