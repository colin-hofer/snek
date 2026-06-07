package main

import (
	"flag"
	"fmt"
	"os"
	"snek"
)

func main() {
	bot := flag.Bool("bot", false, "let the computer play")
	flag.Parse()

	if err := snek.RunWithOptions(snek.Options{Bot: *bot}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
