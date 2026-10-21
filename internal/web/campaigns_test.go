package web

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"atria/internal/games"
	"atria/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const webTestCampaign = `{
	"book": {"slug": "test-book", "title": "Test Book", "author": "A. Writer", "topics": ["legends"]},
	"campaigns": [{"slug": "the-story", "title": "The Story", "reviewed": true, "passages": [
		"Once upon a time a wise old king lived in a castle by the sea.",
		"Every morning the king walked along the shore and talked to the fishermen.",
		"One day a great storm came, and the king helped the people of the village."
	]}]
}`

func TestCampaignPages(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	file, err := games.ReadImportFile(strings.NewReader(webTestCampaign))
	require.NoError(t, err)
	_, err = games.Import(ctx, db, file, false)
	require.NoError(t, err)
	r := newTestRouter(t, NewServer(db))
	asUser(t, user)

	list := getWithCookie(r, "/games", nil)
	require.Equal(t, http.StatusOK, list.Code)
	body := list.Body.String()
	assert.Contains(t, body, "The Story")
	assert.Contains(t, body, "0 of 3 solved")
	assert.Contains(t, body, ">Legends</a>", "the topic filter shows the topics")

	single := getWithCookie(r, "/games?tab=single", nil).Body.String()
	assert.NotContains(t, single, "The Story", "campaign passages are not single puzzles")
	assert.NotContains(t, getWithCookie(r, "/games?tab=single&topic=legends", nil).Body.String(), "A divided community")

	campaigns, err := games.ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	campaignID := campaigns[0].ID.String()
	page := getWithCookie(r, "/games/campaigns/"+campaignID, nil)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Solve the first passage to start the story.")

	var second string
	require.NoError(t, db.QueryRow(`SELECT id FROM game_cryptograms WHERE campaign_id = $1 AND order_index = 1`, campaignID).Scan(&second))
	locked := getWithCookie(r, "/games/cryptograms/"+second, nil)
	assert.Equal(t, http.StatusSeeOther, locked.Code)
	assert.Equal(t, "/games/campaigns/"+campaignID, locked.Header().Get("Location"))

	first := getWithCookie(r, "/games/cryptograms/"+campaigns[0].NextID.String(), nil)
	require.Equal(t, http.StatusOK, first.Code)
	assert.Contains(t, first.Body.String(), "Passage 1 of 3")
	assert.Contains(t, first.Body.String(), "Story so far")

	_, err = db.Exec(`UPDATE game_cryptogram_progress SET completed_at = NOW() WHERE user_id = $1`, user.ID)
	require.NoError(t, err)
	story := getWithCookie(r, "/games/campaigns/"+campaignID, nil).Body.String()
	assert.Contains(t, story, "Once upon a time a wise old king lived in a castle by the sea.")
	assert.Contains(t, story, "Next passage")
	assert.NotRegexp(t, inlineStyle, mainContent(story))
}
