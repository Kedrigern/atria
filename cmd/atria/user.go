package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"atria/internal/core"
	"atria/internal/database"
	"atria/internal/users"
)

var (
	userEmail    string
	userName     string
	userPassword string
	userRole     string
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management",
}

var userAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Creates a new user",
	Run: func(cmd *cobra.Command, args []string) {
		if userEmail == "" || userPassword == "" || userName == "" {
			log.Fatal("ERROR: --email, --name, and --password are required fields")
		}

		role := core.Role(userRole)
		if role != core.RoleAdmin && role != core.RoleUser {
			log.Fatalf("ERROR: invalid role '%s'. Must be 'admin' or 'user'", userRole)
		}

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Connection failed: %v", err)
		}
		defer db.Close()

		user, err := users.CreateUser(context.Background(), db, userEmail, userName, userPassword, role)
		if err != nil {
			log.Fatalf("Failed to create user: %v", err)
		}

		fmt.Printf("✅ User successfully created!\nID: %s\nEmail: %s\nRole: %s\n", user.ID, user.Email, user.Role)
	},
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "Outputs a tabular list of all users on the instance",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Database connection failed: %v", err)
		}
		defer db.Close()

		userList, err := users.ListUsers(context.Background(), db)
		if err != nil {
			log.Fatalf("Failed to fetch users: %v", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tNAME\tROLE\tCREATED")
		for _, u := range userList {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.ID, u.Email, u.DisplayName, u.Role, u.CreatedAt.Format("2006-01-02 15:04"))
		}
		w.Flush()
	},
}

var userShowCmd = &cobra.Command{
	Use:   "show <email|uuid>",
	Short: "Displays detailed information about a specific user",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Database connection failed: %v", err)
		}
		defer db.Close()

		user, err := core.FindUser(context.Background(), db, identifier)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Printf("--- User Details ---\n")
		fmt.Printf("ID:            %s\n", user.ID)
		fmt.Printf("Email:         %s\n", user.Email)
		fmt.Printf("Display Name:  %s\n", user.DisplayName)
		fmt.Printf("Role:          %s\n", user.Role)
		fmt.Printf("Created At:    %s\n", user.CreatedAt.Format(time.RFC3339))
		if user.LastLoginAt != nil {
			fmt.Printf("Last Login:    %s\n", user.LastLoginAt.Format(time.RFC3339))
		} else {
			fmt.Printf("Last Login:    Never\n")
		}
	},
}

var userRoleCmd = &cobra.Command{
	Use:   "role <email|uuid> <user|admin>",
	Short: "Changes the permission role of an existing user",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		identifier := args[0]
		newRole := core.Role(args[1])

		if newRole != core.RoleAdmin && newRole != core.RoleUser {
			log.Fatalf("ERROR: invalid role '%s'. Must be 'admin' or 'user'", newRole)
		}

		db, err := database.InitDB(os.Getenv("DATABASE_URL"))
		if err != nil {
			log.Fatalf("Database connection failed: %v", err)
		}
		defer db.Close()

		if err := users.UpdateUserRole(context.Background(), db, identifier, newRole); err != nil {
			log.Fatalf("Failed to update role: %v", err)
		}

		fmt.Printf("✅ User %s role successfully updated to '%s'.\n", identifier, newRole)
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userAddCmd, userListCmd, userShowCmd, userRoleCmd)

	userAddCmd.Flags().StringVar(&userEmail, "email", "", "User's email address (required)")
	userAddCmd.Flags().StringVar(&userName, "name", "", "User's display name (required)")
	userAddCmd.Flags().StringVar(&userPassword, "password", "", "User's password (required)")
	userAddCmd.Flags().StringVar(&userRole, "role", "user", "User's role: 'admin' or 'user' (default 'user')")
}
