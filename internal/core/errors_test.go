package core

import (
	"errors"
	"fmt"
	"testing"
)

func TestUserMessage(t *testing.T) {
	base := errors.New("dial tcp: timeout")
	userErr := NewUserError("could not download feed: %w", base)
	wrapped := fmt.Errorf("fetch feed 42: %w", userErr)

	msg, ok := UserMessage(wrapped)
	if !ok || msg != "could not download feed: dial tcp: timeout" {
		t.Errorf("UserMessage = %q, %v", msg, ok)
	}
	if !errors.Is(wrapped, base) {
		t.Error("wrapped cause must stay reachable")
	}

	if _, ok := UserMessage(errors.New("pq: relation does not exist")); ok {
		t.Error("plain errors must not be treated as user-facing")
	}
	if _, ok := UserMessage(nil); ok {
		t.Error("nil is not a user error")
	}
}
