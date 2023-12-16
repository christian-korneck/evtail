# gowinlog

[![Go Build](https://github.com/ofcoursedude/gowinlog/actions/workflows/go.yml/badge.svg)](https://github.com/ofcoursedude/gowinlog/actions/workflows/go.yml)
[![CodeQL](https://github.com/ofcoursedude/gowinlog/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/ofcoursedude/gowinlog/actions/workflows/codeql-analysis.yml)

Go library for subscribing to the Windows Event Log.

Godocs
=======

[![PkgGoDev](https://pkg.go.dev/badge/github.com/ofcoursedude/gowinlog)](https://pkg.go.dev/github.com/ofcoursedude/gowinlog)

Installation
=======

just go get the thing

Features
========

- Includes wrapper for wevtapi.dll, and a high level API
- Supports bookmarks for resuming consumption
- Filter events using XPath expressions 

Usage
=======

``` Go
package main

import (
	"fmt"
	"time"

	winlog "github.com/ofcoursedude/gowinlog"
)

func main() {
	fmt.Println("Starting...")
	watcher, err := winlog.NewWinLogWatcher()
	if err != nil {
		fmt.Printf("Couldn't create watcher: %v\n", err)
		return
	}
	// Recieve any future messages on the Application channel
	// "*" doesn't filter by any fields of the event
	watcher.SubscribeFromNow("Application", "*")
	for {
		select {
		case evt := <-watcher.Event():
			// Print the event struct
			// fmt.Printf("\nEvent: %v\n", evt)
			// or print basic output
			fmt.Printf("\n%s: %s: %s\n", evt.LevelText, evt.ProviderName, evt.Msg)
		case err := <-watcher.Error():
			fmt.Printf("\nError: %v\n\n", err)
		default:
			// If no event is waiting, need to wait or do something else, otherwise
			// the the app fails on deadlock.
			<-time.After(1 * time.Millisecond)
		}
	}
}
```

Low-level API
------

`winevt.go` provides wrappers around the relevant functions in `wevtapi.dll`.
