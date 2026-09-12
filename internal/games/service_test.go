package games

import (
	"fmt"
	"testing"
)

func TestNewMappingUsesEveryDistinctLetterOnce(t *testing.T) {
	mapping, err := newMapping("A CAB, BABY!")
	if err != nil {
		t.Fatalf("newMapping() error: %v", err)
	}

	seenLetters := make(map[string]bool)
	for code, letter := range mapping {
		if code == "" {
			t.Error("mapping contains an empty code")
		}
		seenLetters[letter] = true
	}
	for _, letter := range []string{"A", "B", "C", "Y"} {
		if !seenLetters[letter] {
			t.Errorf("mapping does not contain %q", letter)
		}
	}
	if got, want := len(mapping), 4; got != want {
		t.Errorf("mapping length = %d, want %d", got, want)
	}
}

func TestTokensForHidesSolutionAndPreservesFormatting(t *testing.T) {
	mapping := map[string]string{"1": "A", "2": "B"}
	tokens := tokensFor("A B!", mapping, map[string]string{"1": "A"})
	if got, want := len(tokens), 4; got != want {
		t.Fatalf("token count = %d, want %d", got, want)
	}
	if !tokens[0].IsLetter || tokens[0].Code != 1 || tokens[0].Guess != "A" {
		t.Errorf("first token = %#v, want solved A with code 1", tokens[0])
	}
	if got := tokens[1].Character; got != " " {
		t.Errorf("space token = %q, want space", got)
	}
	if !tokens[2].IsLetter || tokens[2].Code != 2 || tokens[2].Guess != "" {
		t.Errorf("third token = %#v, want unsolved letter with code 2", tokens[2])
	}
	if got := tokens[3].Character; got != "!" {
		t.Errorf("punctuation token = %q, want !", got)
	}
	for _, token := range tokens {
		if token.IsLetter && token.Guess == "B" {
			t.Errorf("unresolved solution was exposed in token %s", fmt.Sprint(token))
		}
	}
}

func TestWordsForPreservesWordBoundaries(t *testing.T) {
	mapping := map[string]string{"1": "A", "2": "B"}
	words := wordsFor("A B!", mapping, map[string]string{})
	if got, want := len(words), 2; got != want {
		t.Fatalf("word count = %d, want %d", got, want)
	}
	if got, want := len(words[0]), 1; got != want {
		t.Errorf("first word token count = %d, want %d", got, want)
	}
	if got, want := len(words[1]), 2; got != want {
		t.Errorf("second word token count = %d, want %d", got, want)
	}
	if got := words[1][1].Character; got != "!" {
		t.Errorf("word punctuation = %q, want !", got)
	}
}

func TestInitialGuessesRevealOnlyConfiguredLetters(t *testing.T) {
	mapping := map[string]string{"1": "A", "2": "B", "3": "C"}
	guesses := initialGuesses(mapping, []string{"B", "X"})
	if got, want := guesses["2"], "B"; got != want {
		t.Errorf("revealed guess = %q, want %q", got, want)
	}
	if len(guesses) != 1 {
		t.Errorf("guess count = %d, want 1", len(guesses))
	}
}
