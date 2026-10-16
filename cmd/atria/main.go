package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func main() {

	// A missing .env is fine: configuration may come from the environment.
	_ = godotenv.Load()

	// Execute the root command and capture any errors
	err := rootCmd.Execute()

	// GUARANTEED CLEANUP:
	// Regardless of success or failure, ensure the DB connection is closed cleanly.
	if app != nil && app.DB != nil {
		app.DB.Close()
	}

	// Handle the captured error after cleanup
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
