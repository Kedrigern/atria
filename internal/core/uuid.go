package core

import (
	"log"

	"github.com/gofrs/uuid/v5"
)

// NewUUID generates a new chronological UUIDv7.
// It panics on error because UUID generation failure means catastrophic system entropy loss.
func NewUUID() uuid.UUID {
	/// Explicitly using version 7
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatalf("CRITICAL: Failed to generate UUIDv7: %v", err)
	}
	return id
}

// ParseUUID safely converts a string to a UUID object.
func ParseUUID(s string) (uuid.UUID, error) {
	return uuid.FromString(s)
}
