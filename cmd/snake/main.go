package main

import (
	"fmt"
	"os"

	"sandbox"
)

func main() {
	if err := snake.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
