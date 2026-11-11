package main

import (
	"fmt"
	"log"
	"os"
	"tbc/internal/commands"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) < 2 {
		if err := commands.RunInteractive(); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
