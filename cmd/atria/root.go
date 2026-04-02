package main

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var Version = "v0.0.1-dev"

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "atria",
	Short:   "Atria - Personal Mind Palace CLI",
	Long:    `Atria is a unified tool for managing your knowledge base, reading list, and RSS feeds.`,
	Version: Version,
}

func init() {
	// Load variables from the .env file globally before any command runs
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, relying on system environment variables.")
	}
}
