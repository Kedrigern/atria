// Package games manages cryptogram catalog entries and private player progress.
package games

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode"

	"atria/internal/core"

	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

// Summary is a catalog entry with the current user's completion state.
type Summary struct {
	ID              uuid.UUID
	Title           string
	Category        *string
	IntroText       *string
	DifficultyScore int
	CompletedAt     *time.Time
	Topics          []string
}

// Token is one player-visible character in a cryptogram.
type Token struct {
	Character string
	Code      int
	Guess     string
	IsLetter  bool
}

// Puzzle is a safe, player-visible projection of a cryptogram attempt.
type Puzzle struct {
	ID              uuid.UUID
	Title           string
	Category        *string
	IntroText       *string
	CompletionText  *string
	DifficultyScore int
	Words           [][]Token
	MovesCount      int
	ErrorsMade      int
	HintsUsed       int
	RecentMoves     []Move
	CompletedAt     *time.Time
	// Campaign is set for a passage of a campaign.
	Campaign *CampaignContext
}

// Move is a recent accepted or rejected letter assignment.
type Move struct {
	Code      int
	Letter    string
	IsCorrect bool
	IsHint    bool
}

// ValidationError describes a rejected player action that is safe to show in the UI.
type ValidationError struct {
	Message string
}

// NotFoundError identifies a missing cryptogram definition.
type NotFoundError struct{}

// Error returns the player-facing validation message.
func (e *ValidationError) Error() string {
	return e.Message
}

func validationError(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// Error returns a generic message that does not disclose catalog details.
func (e *NotFoundError) Error() string {
	return "cryptogram not found"
}

// List returns the single puzzles (not the passages of campaigns) and their
// completion state for a user. An empty topic returns all of them.
func List(ctx context.Context, db *sql.DB, userID uuid.UUID, topic string) ([]Summary, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.id, c.title, c.category, c.intro_text, c.difficulty_score, p.completed_at, c.topics
		FROM game_cryptograms c
		LEFT JOIN game_cryptogram_progress p ON p.cryptogram_id = c.id AND p.user_id = $1
		WHERE c.campaign_id IS NULL AND ($2 = '' OR $2 = ANY(c.topics))
		ORDER BY (p.completed_at IS NOT NULL), c.category NULLS LAST, c.order_index NULLS LAST, c.title
	`, userID, topic)
	if err != nil {
		return nil, fmt.Errorf("list cryptograms: %w", err)
	}
	defer rows.Close()

	var puzzles []Summary
	for rows.Next() {
		var puzzle Summary
		if err := rows.Scan(&puzzle.ID, &puzzle.Title, &puzzle.Category, &puzzle.IntroText, &puzzle.DifficultyScore, &puzzle.CompletedAt, pq.Array(&puzzle.Topics)); err != nil {
			return nil, fmt.Errorf("scan cryptogram: %w", err)
		}
		puzzles = append(puzzles, puzzle)
	}
	return puzzles, rows.Err()
}

// GetPuzzle creates or loads a user's stable attempt and returns its safe projection.
func GetPuzzle(ctx context.Context, db *sql.DB, userID, cryptogramID uuid.UUID) (*Puzzle, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin cryptogram attempt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	puzzle, plainText, reveals, err := loadDefinition(ctx, tx, cryptogramID)
	if err != nil {
		return nil, err
	}
	if puzzle.Campaign, err = campaignContext(ctx, tx, userID, cryptogramID); err != nil {
		return nil, err
	}

	mapping, guesses, moves, errorsMade, hintsUsed, completedAt, found, err := loadProgress(ctx, tx, userID, cryptogramID)
	if err != nil {
		return nil, err
	}
	created := false
	if !found {
		mapping, err = newMapping(plainText)
		if err != nil {
			return nil, err
		}
		guesses = initialGuesses(mapping, reveals)
		mappingJSON, err := json.Marshal(mapping)
		if err != nil {
			return nil, fmt.Errorf("encode cipher mapping: %w", err)
		}
		guessesJSON, err := json.Marshal(guesses)
		if err != nil {
			return nil, fmt.Errorf("encode initial guesses: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO game_cryptogram_progress (id, user_id, cryptogram_id, cipher_mapping, guesses, hints_used)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, cryptogram_id) DO NOTHING
		`, core.NewUUID(), userID, cryptogramID, mappingJSON, guessesJSON, len(guesses))
		if err != nil {
			return nil, fmt.Errorf("create cryptogram attempt: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("confirm cryptogram attempt creation: %w", err)
		}
		if rows == 1 {
			created = true
			hintsUsed = len(guesses)
		} else {
			mapping, guesses, moves, errorsMade, hintsUsed, completedAt, found, err = loadProgress(ctx, tx, userID, cryptogramID)
			if err != nil {
				return nil, fmt.Errorf("load concurrently created cryptogram attempt: %w", err)
			}
			if !found {
				return nil, fmt.Errorf("concurrently created cryptogram attempt is unavailable")
			}
		}
	}
	if !created {
		revealedGuesses := initialGuesses(mapping, reveals)
		if mergeGuesses(guesses, revealedGuesses) || hintsUsed < len(revealedGuesses) {
			guessesJSON, err := json.Marshal(guesses)
			if err != nil {
				return nil, fmt.Errorf("encode revealed guesses: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE game_cryptogram_progress
				SET guesses = $1, hints_used = GREATEST(hints_used, $2), updated_at = CURRENT_TIMESTAMP
				WHERE user_id = $3 AND cryptogram_id = $4
			`, guessesJSON, len(revealedGuesses), userID, cryptogramID); err != nil {
				return nil, fmt.Errorf("apply revealed letters: %w", err)
			}
			hintsUsed = len(revealedGuesses)
		}
	}

	recentMoves, err := listRecentMoves(ctx, tx, userID, cryptogramID)
	if err != nil {
		return nil, err
	}
	puzzle.Words = wordsFor(plainText, mapping, guesses)
	puzzle.MovesCount = moves
	puzzle.ErrorsMade = errorsMade
	puzzle.HintsUsed = hintsUsed
	puzzle.RecentMoves = recentMoves
	puzzle.CompletedAt = completedAt
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit cryptogram attempt: %w", err)
	}
	return puzzle, nil
}

// Guess validates and records a correct one-to-one code assignment.
func Guess(ctx context.Context, db *sql.DB, userID, cryptogramID uuid.UUID, code int, letter string) error {
	if code < 1 {
		return validationError("select a cipher number")
	}
	letter = strings.ToUpper(strings.TrimSpace(letter))
	if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'Z' {
		return validationError("enter one letter from A to Z")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cryptogram guess: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	mapping, guesses, _, _, _, completedAt, found, err := loadProgress(ctx, tx, userID, cryptogramID)
	if err != nil {
		return err
	}
	if !found {
		return validationError("open the puzzle before submitting a guess")
	}
	if completedAt != nil {
		return validationError("this puzzle is already complete")
	}
	codeKey := fmt.Sprintf("%d", code)
	solution, exists := mapping[codeKey]
	if !exists {
		return validationError("unknown cipher number")
	}
	if existing := guesses[codeKey]; existing != "" {
		return validationError("this cipher number is already solved")
	}
	for assignedCode, assignedLetter := range guesses {
		if assignedCode != codeKey && assignedLetter == letter {
			return validationError("%s is already assigned to another number", letter)
		}
	}
	if solution != letter {
		if _, err := tx.ExecContext(ctx, `
			UPDATE game_cryptogram_progress
			SET errors_made = errors_made + 1, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $1 AND cryptogram_id = $2
		`, userID, cryptogramID); err != nil {
			return fmt.Errorf("record cryptogram mistake: %w", err)
		}
		if err := recordMove(ctx, tx, userID, cryptogramID, code, letter, false, false); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit cryptogram mistake: %w", err)
		}
		return validationError("%s is not the letter for number %d.", letter, code)
	}

	guesses[codeKey] = letter
	guessesJSON, err := json.Marshal(guesses)
	if err != nil {
		return fmt.Errorf("encode guesses: %w", err)
	}
	completed := allSolved(mapping, guesses)
	if completed {
		_, err = tx.ExecContext(ctx, `
			UPDATE game_cryptogram_progress
			SET guesses = $1, moves_count = moves_count + 1, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $2 AND cryptogram_id = $3
		`, guessesJSON, userID, cryptogramID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE game_cryptogram_progress
			SET guesses = $1, moves_count = moves_count + 1, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $2 AND cryptogram_id = $3
		`, guessesJSON, userID, cryptogramID)
	}
	if err != nil {
		return fmt.Errorf("save cryptogram guess: %w", err)
	}
	if err := recordMove(ctx, tx, userID, cryptogramID, code, letter, true, false); err != nil {
		return err
	}
	return tx.Commit()
}

// Hint reveals one unresolved cipher letter for the requesting user.
func Hint(ctx context.Context, db *sql.DB, userID, cryptogramID uuid.UUID) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cryptogram hint: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	mapping, guesses, _, _, _, completedAt, found, err := loadProgress(ctx, tx, userID, cryptogramID)
	if err != nil {
		return err
	}
	if !found || completedAt != nil {
		return core.NewUserError("cryptogram is not available for hints")
	}

	codes := make([]int, 0)
	for code := range mapping {
		if guesses[code] != "" {
			continue
		}
		var parsedCode int
		_, _ = fmt.Sscanf(code, "%d", &parsedCode)
		codes = append(codes, parsedCode)
	}
	if len(codes) == 0 {
		return core.NewUserError("no unresolved letters remain")
	}
	sort.Ints(codes)
	codeKey := fmt.Sprintf("%d", codes[0])
	guesses[codeKey] = mapping[codeKey]
	guessesJSON, err := json.Marshal(guesses)
	if err != nil {
		return fmt.Errorf("encode hint guesses: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE game_cryptogram_progress
		SET guesses = $1, hints_used = hints_used + 1,
			completed_at = CASE WHEN $2 THEN CURRENT_TIMESTAMP ELSE completed_at END,
			updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $3 AND cryptogram_id = $4
	`, guessesJSON, allSolved(mapping, guesses), userID, cryptogramID)
	if err != nil {
		return fmt.Errorf("save cryptogram hint: %w", err)
	}
	if err := recordMove(ctx, tx, userID, cryptogramID, codes[0], mapping[codeKey], true, true); err != nil {
		return err
	}
	return tx.Commit()
}

// Reset deletes only the requesting user's progress for a puzzle.
func Reset(ctx context.Context, db *sql.DB, userID, cryptogramID uuid.UUID) error {
	result, err := db.ExecContext(ctx, `DELETE FROM game_cryptogram_progress WHERE user_id = $1 AND cryptogram_id = $2`, userID, cryptogramID)
	if err != nil {
		return fmt.Errorf("reset cryptogram: %w", err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows == 0 {
		return core.NewUserError("cryptogram attempt not found")
	}
	return nil
}

func loadDefinition(ctx context.Context, tx *sql.Tx, cryptogramID uuid.UUID) (*Puzzle, string, []string, error) {
	var puzzle Puzzle
	var plainText string
	var reveals []string
	err := tx.QueryRowContext(ctx, `
		SELECT id, title, category, plain_text, intro_text, completion_text, initial_revealed_letters, difficulty_score
		FROM game_cryptograms WHERE id = $1
	`, cryptogramID).Scan(&puzzle.ID, &puzzle.Title, &puzzle.Category, &plainText, &puzzle.IntroText, &puzzle.CompletionText, pq.Array(&reveals), &puzzle.DifficultyScore)
	if err == sql.ErrNoRows {
		return nil, "", nil, &NotFoundError{}
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("load cryptogram: %w", err)
	}
	return &puzzle, strings.ToUpper(plainText), reveals, nil
}

func loadProgress(ctx context.Context, tx *sql.Tx, userID, cryptogramID uuid.UUID) (map[string]string, map[string]string, int, int, int, *time.Time, bool, error) {
	var mappingJSON, guessesJSON []byte
	var moves, errorsMade, hintsUsed int
	var completedAt *time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT cipher_mapping, guesses, moves_count, errors_made, hints_used, completed_at
		FROM game_cryptogram_progress WHERE user_id = $1 AND cryptogram_id = $2 FOR UPDATE
	`, userID, cryptogramID).Scan(&mappingJSON, &guessesJSON, &moves, &errorsMade, &hintsUsed, &completedAt)
	if err == sql.ErrNoRows {
		return nil, nil, 0, 0, 0, nil, false, nil
	}
	if err != nil {
		return nil, nil, 0, 0, 0, nil, false, fmt.Errorf("load cryptogram progress: %w", err)
	}
	mapping := map[string]string{}
	guesses := map[string]string{}
	if err := json.Unmarshal(mappingJSON, &mapping); err != nil {
		return nil, nil, 0, 0, 0, nil, false, fmt.Errorf("decode cipher mapping: %w", err)
	}
	if err := json.Unmarshal(guessesJSON, &guesses); err != nil {
		return nil, nil, 0, 0, 0, nil, false, fmt.Errorf("decode guesses: %w", err)
	}
	return mapping, guesses, moves, errorsMade, hintsUsed, completedAt, true, nil
}

func recordMove(ctx context.Context, tx *sql.Tx, userID, cryptogramID uuid.UUID, code int, letter string, isCorrect, isHint bool) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO game_cryptogram_moves (id, progress_id, code, letter, is_correct, is_hint)
		SELECT $1, id, $2, $3, $4, $5
		FROM game_cryptogram_progress
		WHERE user_id = $6 AND cryptogram_id = $7
	`, core.NewUUID(), code, letter, isCorrect, isHint, userID, cryptogramID)
	if err != nil {
		return fmt.Errorf("record cryptogram move: %w", err)
	}
	return nil
}

func listRecentMoves(ctx context.Context, tx *sql.Tx, userID, cryptogramID uuid.UUID) ([]Move, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.code, m.letter, m.is_correct, m.is_hint
		FROM game_cryptogram_moves m
		JOIN game_cryptogram_progress p ON p.id = m.progress_id
		WHERE p.user_id = $1 AND p.cryptogram_id = $2
		ORDER BY m.created_at DESC
		LIMIT 5
	`, userID, cryptogramID)
	if err != nil {
		return nil, fmt.Errorf("list cryptogram moves: %w", err)
	}
	defer rows.Close()
	moves := make([]Move, 0)
	for rows.Next() {
		var move Move
		if err := rows.Scan(&move.Code, &move.Letter, &move.IsCorrect, &move.IsHint); err != nil {
			return nil, fmt.Errorf("scan cryptogram move: %w", err)
		}
		moves = append(moves, move)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cryptogram moves: %w", err)
	}
	return moves, nil
}

func newMapping(plainText string) (map[string]string, error) {
	letters := map[rune]struct{}{}
	for _, character := range plainText {
		if character >= 'A' && character <= 'Z' {
			letters[character] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(letters))
	for letter := range letters {
		ordered = append(ordered, string(letter))
	}
	sort.Strings(ordered)
	for index := len(ordered) - 1; index > 0; index-- {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(index+1)))
		if err != nil {
			return nil, fmt.Errorf("shuffle cipher mapping: %w", err)
		}
		ordered[index], ordered[randomIndex.Int64()] = ordered[randomIndex.Int64()], ordered[index]
	}
	mapping := make(map[string]string, len(ordered))
	for code, letter := range ordered {
		mapping[fmt.Sprintf("%d", code+1)] = letter
	}
	return mapping, nil
}

func initialGuesses(mapping map[string]string, reveals []string) map[string]string {
	guesses := make(map[string]string)
	for _, reveal := range reveals {
		reveal = strings.ToUpper(reveal)
		for code, letter := range mapping {
			if letter == reveal {
				guesses[code] = letter
			}
		}
	}
	return guesses
}

func tokensFor(plainText string, mapping, guesses map[string]string) []Token {
	codeByLetter := make(map[string]int, len(mapping))
	for code, letter := range mapping {
		var parsedCode int
		_, _ = fmt.Sscanf(code, "%d", &parsedCode)
		codeByLetter[letter] = parsedCode
	}
	tokens := make([]Token, 0, len(plainText))
	for _, character := range plainText {
		if character >= 'A' && character <= 'Z' {
			code := codeByLetter[string(character)]
			tokens = append(tokens, Token{Code: code, Guess: guesses[fmt.Sprintf("%d", code)], IsLetter: true})
			continue
		}
		if unicode.IsSpace(character) {
			tokens = append(tokens, Token{Character: " ", IsLetter: false})
			continue
		}
		tokens = append(tokens, Token{Character: string(character), IsLetter: false})
	}
	return tokens
}

func mergeGuesses(guesses, revealedGuesses map[string]string) bool {
	changed := false
	for code, letter := range revealedGuesses {
		if guesses[code] == "" {
			guesses[code] = letter
			changed = true
		}
	}
	return changed
}

func wordsFor(plainText string, mapping, guesses map[string]string) [][]Token {
	tokens := tokensFor(plainText, mapping, guesses)
	words := make([][]Token, 0)
	word := make([]Token, 0)
	for _, token := range tokens {
		if !token.IsLetter && token.Character == " " {
			if len(word) > 0 {
				words = append(words, word)
				word = make([]Token, 0)
			}
			continue
		}
		word = append(word, token)
	}
	if len(word) > 0 {
		words = append(words, word)
	}
	return words
}

func allSolved(mapping, guesses map[string]string) bool {
	for code, letter := range mapping {
		if guesses[code] != letter {
			return false
		}
	}
	return true
}
