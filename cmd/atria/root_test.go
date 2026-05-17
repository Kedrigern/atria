package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type DummyItem struct {
	ID   string
	Name string
}

func TestResolveSingle(t *testing.T) {
	formatFunc := func(item DummyItem) string {
		return fmt.Sprintf("%s - %s", item.ID, item.Name)
	}

	t.Run("Propagates underlying database error", func(t *testing.T) {
		dbErr := errors.New("database connection lost")
		res, err := resolveSingle("test-id", []DummyItem{}, dbErr, "dummy", formatFunc)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Contains(t, err.Error(), "dummy search failed")
		assert.Contains(t, err.Error(), "database connection lost")
	})

	t.Run("Returns error when no items found", func(t *testing.T) {
		res, err := resolveSingle("unknown-id", []DummyItem{}, nil, "dummy", formatFunc)

		assert.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, "no dummy found matching 'unknown-id'", err.Error())
	})

	t.Run("Returns the item when exactly one is found", func(t *testing.T) {
		items := []DummyItem{
			{ID: "123", Name: "Perfect Match"},
		}
		res, err := resolveSingle("123", items, nil, "dummy", formatFunc)

		assert.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, "123", res.ID)
		assert.Equal(t, "Perfect Match", res.Name)
	})

	t.Run("Returns formatted ambiguous error for multiple matches", func(t *testing.T) {
		items := []DummyItem{
			{ID: "123", Name: "First Item"},
			{ID: "456", Name: "Second Item"},
		}

		res, err := resolveSingle("Item", items, nil, "dummy", formatFunc)

		assert.Error(t, err)
		assert.Nil(t, res)

		errMsg := err.Error()
		assert.Contains(t, errMsg, "Ambiguous dummy identifier 'Item'")
		assert.Contains(t, errMsg, "Please be more specific")
		assert.Contains(t, errMsg, "123 - First Item")
		assert.Contains(t, errMsg, "456 - Second Item")
	})
}
