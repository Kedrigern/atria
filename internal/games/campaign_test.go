package games

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"atria/internal/testutil"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPassages = []string{
	"Many years ago there lived in England a wise and good king whose name was Alfred.",
	"The woman was baking some cakes upon the hearth, and she asked the king to watch them.",
	"The cakes were smoking on the hearth. They were burned to a crisp. Ah, how angry she was!",
}

func testFile(passages ...string) string {
	quoted := make([]string, len(passages))
	for i, p := range passages {
		quoted[i] = `"` + p + `"`
	}
	return `{
		"book": {"slug": "fifty-famous-stories", "title": "Fifty Famous Stories Retold", "author": "James Baldwin",
		         "source_url": "https://www.gutenberg.org/ebooks/18442", "license": "Public domain in the USA", "topics": ["history"]},
		"campaigns": [{"slug": "king-alfred", "title": "King Alfred and the Cakes", "reviewed": true, "topics": ["legends"],
		               "passages": [` + strings.Join(quoted, ",") + `]}]
	}`
}

func mustRead(t *testing.T, content string) *ImportFile {
	t.Helper()
	file, err := ReadImportFile(strings.NewReader(content))
	require.NoError(t, err)
	return file
}

func TestReadImportFileRejectsBadCampaigns(t *testing.T) {
	cases := map[string]string{
		"not reviewed":   strings.Replace(testFile(testPassages...), `"reviewed": true`, `"reviewed": false`, 1),
		"two passages":   testFile(testPassages[:2]...),
		"short passage":  testFile(testPassages[0], testPassages[1], "Too short."),
		"letter outside": testFile(testPassages[0], testPassages[1], "The Greek letter λ is not in the alphabet of this game at all."),
		"unknown field":  strings.Replace(testFile(testPassages...), `"reviewed"`, `"reviwed"`, 1),
	}
	for name, content := range cases {
		_, err := ReadImportFile(strings.NewReader(content))
		assert.Error(t, err, name)
	}
	mustRead(t, testFile(testPassages...))
}

func TestToASCII(t *testing.T) {
	assert.Equal(t, "Caesar said: \"Veni, vidi, vici\" - it's true.", ToASCII("Cæsar said: “Veni, vidi, vici” — it’s true."))
	assert.Equal(t, "Prilis zlutoucky kun", ToASCII("Příliš žluťoučký kůň"))
}

func TestCampaignOpensPassagesInOrder(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	_, err := Import(ctx, db, mustRead(t, testFile(testPassages...)), false)
	require.NoError(t, err)

	campaigns, err := ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	require.Len(t, campaigns, 1)
	campaign := campaigns[0]
	assert.Equal(t, 3, campaign.Total)
	assert.Equal(t, 0, campaign.Solved)
	assert.Equal(t, []string{"history", "legends"}, campaign.Topics)
	require.NotNil(t, campaign.NextID)

	var ids []string
	rows, err := db.Query(`SELECT id FROM game_cryptograms WHERE campaign_id = $1 ORDER BY order_index`, campaign.ID)
	require.NoError(t, err)
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Close())
	require.Len(t, ids, 3)
	assert.Equal(t, ids[0], campaign.NextID.String())

	// The second passage is locked until the first is solved.
	second := *campaign.NextID
	require.NoError(t, second.Parse(ids[1]))
	_, err = GetPuzzle(ctx, db, user.ID, second)
	var locked *LockedError
	require.True(t, errors.As(err, &locked), "got %v", err)
	assert.Equal(t, campaign.ID, locked.CampaignID)

	first, err := GetPuzzle(ctx, db, user.ID, *campaign.NextID)
	require.NoError(t, err)
	require.NotNil(t, first.Campaign)
	assert.Equal(t, 1, first.Campaign.Position)
	assert.Equal(t, 3, first.Campaign.Total)
	_, err = db.Exec(`UPDATE game_cryptogram_progress SET completed_at = NOW() WHERE user_id = $1 AND cryptogram_id = $2`, user.ID, first.ID)
	require.NoError(t, err)

	opened, err := GetPuzzle(ctx, db, user.ID, second)
	require.NoError(t, err)
	assert.Equal(t, 2, opened.Campaign.Position)

	page, err := GetCampaign(ctx, db, user.ID, campaign.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Solved)
	assert.Equal(t, []string{testPassages[0]}, page.Story, "the story so far keeps the normal case")
	assert.Equal(t, second, *page.NextID)

	// Campaign passages are not single puzzles.
	single, err := List(ctx, db, user.ID, "")
	require.NoError(t, err)
	for _, s := range single {
		assert.NotContains(t, s.Title, "King Alfred")
	}
	filtered, err := ListCampaigns(ctx, db, user.ID, "quotes")
	require.NoError(t, err)
	assert.Empty(t, filtered)
}

func TestImportKeepsUnchangedAndNeedsReplaceForChanges(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	file := mustRead(t, testFile(testPassages...))
	_, err := Import(ctx, db, file, false)
	require.NoError(t, err)

	plan, err := Import(ctx, db, file, false)
	require.NoError(t, err)
	assert.Equal(t, PlanUnchanged, plan[0].State)

	campaigns, err := ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	_, err = GetPuzzle(ctx, db, user.ID, *campaigns[0].NextID)
	require.NoError(t, err)

	changed := mustRead(t, testFile(testPassages[0], testPassages[1], "The king was so hungry that he did not mind the angry words of the woman."))
	plan, err = Import(ctx, db, changed, false)
	require.Error(t, err)
	assert.Equal(t, PlanChanged, plan[0].State)
	assert.Equal(t, 1, plan[0].Players)

	_, err = Import(ctx, db, changed, true)
	require.NoError(t, err)
	var progress int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM game_cryptogram_progress WHERE user_id = $1`, user.ID).Scan(&progress))
	assert.Equal(t, 0, progress, "--replace removes the progress on the changed campaign")
}

// The campaign files in the repository must pass the same checks as an import.
func TestShippedCampaignFilesAreValid(t *testing.T) {
	paths, err := filepath.Glob("../../campaigns/*.json")
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		f, err := os.Open(path)
		require.NoError(t, err)
		_, err = ReadImportFile(f)
		f.Close()
		assert.NoError(t, err, path)
	}
}

const novelFile = `{
	"book": {"slug": "test-novel", "title": "Test Novel", "author": "A. Writer", "in_order": true},
	"campaigns": [
		{"slug": "chapter-one", "title": "Chapter One", "reviewed": true, "passages": [
			"Once upon a time a wise old king lived in a castle by the sea.",
			"Every morning the king walked along the shore and talked to the fishermen.",
			"One day a great storm came, and the king helped the people of the village."]},
		{"slug": "chapter-two", "title": "Chapter Two", "reviewed": true, "passages": [
			"After the storm the fishermen built new boats for the whole village.",
			"The king gave them wood from the old forest behind the castle walls.",
			"In the spring the boats sailed out again, and the nets were full of fish."]}
	]
}`

func solveCampaign(t *testing.T, db *sql.DB, userID uuid.UUID, campaignID uuid.UUID) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO game_cryptogram_progress (id, user_id, cryptogram_id, cipher_mapping, completed_at)
		SELECT gen_random_uuid(), $1, id, '{}'::jsonb, NOW() FROM game_cryptograms WHERE campaign_id = $2
		ON CONFLICT (user_id, cryptogram_id) DO UPDATE SET completed_at = NOW()`, userID, campaignID)
	require.NoError(t, err)
}

func TestNovelOpensChaptersInOrder(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	_, err := Import(ctx, db, mustRead(t, novelFile), false)
	require.NoError(t, err)

	campaigns, err := ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	require.Len(t, campaigns, 2)
	assert.False(t, campaigns[0].Locked())
	assert.Equal(t, "Chapter One", campaigns[1].LockedBy, "the second chapter waits for the first")

	_, err = GetPuzzle(ctx, db, user.ID, *campaigns[1].NextID)
	var locked *LockedError
	require.True(t, errors.As(err, &locked), "the first passage of a locked chapter is locked, got %v", err)

	book, err := GetBook(ctx, db, user.ID, "test-novel")
	require.NoError(t, err)
	assert.True(t, book.InOrder)
	assert.Equal(t, 6, book.Total)
	assert.Equal(t, *campaigns[0].NextID, *book.NextID)

	solveCampaign(t, db, user.ID, campaigns[0].ID)
	campaigns, err = ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	assert.False(t, campaigns[1].Locked(), "the second chapter opens after the first is solved")
	_, err = GetPuzzle(ctx, db, user.ID, *campaigns[1].NextID)
	require.NoError(t, err)

	book, err = GetBook(ctx, db, user.ID, "test-novel")
	require.NoError(t, err)
	assert.Equal(t, 3, book.Solved)
	assert.Equal(t, 1, book.Current, "the current chapter is the second")
	assert.Len(t, book.Chapters[0].Story, 3, "the story of the solved chapter is on the book page")
}

func TestImportAllowsLongCampaignsAndKeepsOrder(t *testing.T) {
	db, user := testutil.SetupTestDB(t)
	ctx := context.Background()
	passages := make([]string, 12)
	for i := range passages {
		passages[i] = testPassages[i%len(testPassages)]
	}
	_, err := Import(ctx, db, mustRead(t, testFile(passages...)), false)
	require.NoError(t, err, "a campaign may have more than 10 passages")

	// A collection keeps the free order: no campaign is locked.
	_, err = Import(ctx, db, mustRead(t, novelFile), false)
	require.NoError(t, err)
	campaigns, err := ListCampaigns(ctx, db, user.ID, "")
	require.NoError(t, err)
	for _, c := range campaigns {
		if c.BookSlug == "fifty-famous-stories" {
			assert.False(t, c.Locked())
			assert.Equal(t, 12, c.Total)
		}
	}
}
