package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"atria/internal/core"
	"atria/internal/games"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleGameList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	puzzles, err := games.List(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to load cryptograms: "+err.Error())
		return
	}
	s.render(c, "games_list.html", gin.H{"Puzzles": puzzles})
}

func (s *Server) handleCryptogramPlay(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		return
	}
	puzzle, err := games.GetPuzzle(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		var notFound *games.NotFoundError
		if errors.As(err, &notFound) {
			s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		} else {
			s.renderError(c, http.StatusInternalServerError, "Failed to load cryptogram")
		}
		return
	}
	s.render(c, "cryptogram_play.html", gin.H{"Puzzle": puzzle})
}

func wantsJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/json")
}

func (s *Server) handleCryptogramGuess(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		return
	}
	var code int
	if _, err := fmt.Sscanf(c.PostForm("code"), "%d", &code); err != nil {
		if wantsJSON(c) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid cipher number"})
		} else {
			s.renderError(c, http.StatusBadRequest, "Invalid cipher number")
		}
		return
	}

	guessSucceeded := false
	if err := games.Guess(c.Request.Context(), s.db, user.ID, id, code, c.PostForm("letter")); err != nil {
		var validation *games.ValidationError
		if errors.As(err, &validation) {
			if wantsJSON(c) {
				c.JSON(http.StatusOK, gin.H{"success": false, "error": validation.Message})
				return
			}
		} else {
			if wantsJSON(c) {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to save cryptogram guess"})
			} else {
				s.renderError(c, http.StatusInternalServerError, "Failed to save cryptogram guess")
			}
			return
		}
	} else {
		guessSucceeded = true
		if wantsJSON(c) {
			c.JSON(http.StatusOK, gin.H{"success": true})
			return
		}
	}

	focusIndex := c.PostForm("focus_index")
	if guessSucceeded && c.PostForm("next_focus_index") != "" {
		focusIndex = c.PostForm("next_focus_index")
	}
	url := "/games/cryptograms/" + id.String()
	if focusIndex != "" {
		url += "?focus=" + focusIndex
	}
	c.Redirect(http.StatusSeeOther, url)
}

func (s *Server) handleCryptogramHint(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		return
	}
	if err := games.Hint(c.Request.Context(), s.db, user.ID, id); err != nil {
		s.renderError(c, http.StatusBadRequest, "Unable to reveal a hint")
		return
	}
	c.Redirect(http.StatusSeeOther, "/games/cryptograms/"+id.String())
}

func (s *Server) handleCryptogramReset(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		return
	}
	if err := games.Reset(c.Request.Context(), s.db, user.ID, id); err != nil {
		s.renderError(c, http.StatusNotFound, "Cryptogram attempt not found")
		return
	}
	c.Redirect(http.StatusSeeOther, "/games/cryptograms/"+id.String())
}
