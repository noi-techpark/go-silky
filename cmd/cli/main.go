// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/noi-techpark/go-silky"
)

func main() {
	configPath := flag.String("config", "", "Path to YAML configuration file")
	profilerFlag := flag.Bool("profiler", false, "Enable profiler output (JSON per step)")
	validateFlag := flag.Bool("validate", false, "Only validate configuration without running")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -config flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create crawler (this also validates)
	crawler, validationErrs, err := silky.NewApiCrawler(*configPath)

	// Check validation errors first (these have detailed messages)
	if len(validationErrs) > 0 {
		fmt.Fprintln(os.Stderr, "Configuration validation failed:")
		for _, e := range validationErrs {
			fmt.Fprintf(os.Stderr, "  - %s: %s\n", e.Location, e.Message)
		}
		os.Exit(1)
	}

	// Then check for other errors (file not found, parse errors, etc.)
	if err != nil {
		log.Fatalf("Failed to create crawler: %v", err)
	}

	if *validateFlag {
		fmt.Println("Configuration is valid")
		return
	}

	var wg sync.WaitGroup

	// Enable profiler if requested
	var profilerChan chan silky.StepProfilerData
	if *profilerFlag {
		profilerChan = crawler.EnableProfiler()
		wg.Add(1)

		// Output profiler data as JSON
		go func() {
			defer wg.Done()
			for data := range profilerChan {
				jsonData, err := json.Marshal(data)
				if err != nil {
					log.Printf("Failed to marshal profiler data: %v", err)
					continue
				}
				fmt.Println(string(jsonData))
			}
		}()
	}

	// Handle stream mode if enabled
	var streamChan chan interface{}
	streamDone := make(chan bool)

	if crawler.Config.Stream {
		streamChan = crawler.GetDataStream()

		// Consume stream and output entities
		go func() {
			for entity := range streamChan {
				jsonEntity, err := json.Marshal(entity)
				if err != nil {
					log.Printf("Failed to marshal stream entity: %v", err)
					continue
				}
				if !*profilerFlag {
					fmt.Printf("STREAM: %s\n", string(jsonEntity))
				}
			}
			streamDone <- true
		}()
	}

	// Run crawler
	ctx := context.Background()
	err = crawler.Run(ctx)

	// Wait for stream to finish if in stream mode
	if crawler.Config.Stream {
		close(streamChan)
		<-streamDone
	}

	// Close profilerChan to signal the consumer goroutine to exit
	if *profilerFlag {
		close(profilerChan)
		wg.Wait() // ✅ Wait until all profiler data is consumed
	}

	if err != nil {
		log.Fatalf("Crawl failed: %v", err)
	}

	// Get result from crawler context
	result := crawler.GetData()

	// Output result as JSON (if not in stream mode and not profile mode)
	if !crawler.Config.Stream && !*profilerFlag {
		// In profiler mode without streaming, output final result after profiler data
		jsonResult, err := json.Marshal(result)
		if err != nil {
			log.Fatalf("Failed to marshal result: %v", err)
		}
		fmt.Printf("RESULT: %s\n", string(jsonResult))
	}
}
