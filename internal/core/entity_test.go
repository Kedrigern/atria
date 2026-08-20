package core_test

import (
	"context"
	"strings"
	"testing"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
)

func TestRenameEntityRejectsBlankTitle(t *testing.T) {
	for _, title := range []string{"", " \t\n "} {
		t.Run("blank", func(t *testing.T) {
			err := core.RenameEntity(context.Background(), nil, uuid.Nil, uuid.Nil, title)
			if err == nil {
				t.Fatal("RenameEntity() returned nil error for a blank title")
			}
			if !strings.Contains(err.Error(), "entity title is required") {
				t.Errorf("RenameEntity() error = %q, want title validation error", err)
			}
		})
	}
}
