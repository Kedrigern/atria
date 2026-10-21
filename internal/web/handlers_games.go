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

// bookGroup is one book on the Campaigns tab: open campaigns first, solved last.
type bookGroup struct {
	Title  string
	Author *string
	Open   []games.CampaignSummary
	Solved []games.CampaignSummary
}

func (s *Server) handleGameList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	ctx := c.Request.Context()
	tab := c.Query("tab")
	if tab != "single" {
		tab = "campaigns"
	}
	topic := c.Query("topic")
	topics, err := games.Topics(ctx, s.db, tab == "campaigns")
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load topics", err)
		return
	}
	data := gin.H{"Tab": tab, "Topic": topic, "Topics": topics}

	if tab == "campaigns" {
		campaigns, err := games.ListCampaigns(ctx, s.db, user.ID, topic)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Failed to load campaigns", err)
			return
		}
		var books []*bookGroup
		for _, campaign := range campaigns {
			if len(books) == 0 || books[len(books)-1].Title != campaign.BookTitle {
				books = append(books, &bookGroup{Title: campaign.BookTitle, Author: campaign.BookAuthor})
			}
			book := books[len(books)-1]
			if campaign.Done() {
				book.Solved = append(book.Solved, campaign)
			} else {
				book.Open = append(book.Open, campaign)
			}
		}
		data["Books"] = books
	} else {
		puzzles, err := games.List(ctx, s.db, user.ID, topic)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Failed to load cryptograms", err)
			return
		}
		var open, solved []games.Summary
		for _, puzzle := range puzzles {
			if puzzle.CompletedAt != nil {
				solved = append(solved, puzzle)
			} else {
				open = append(open, puzzle)
			}
		}
		data["Open"], data["Solved"] = open, solved
	}
	s.render(c, "games_list.html", data)
}

func (s *Server) handleCampaign(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Campaign not found")
		return
	}
	campaign, err := games.GetCampaign(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		var notFound *games.NotFoundError
		if errors.As(err, &notFound) {
			s.renderError(c, http.StatusNotFound, "Campaign not found")
		} else {
			s.respondError(c, http.StatusInternalServerError, "Failed to load campaign", err)
		}
		return
	}
	s.render(c, "campaign_detail.html", gin.H{"Campaign": campaign})
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
		var locked *games.LockedError
		switch {
		case errors.As(err, &locked):
			s.setFlash(c, "info", "Solve the earlier passages first.")
			c.Redirect(http.StatusSeeOther, "/games/campaigns/"+locked.CampaignID.String())
		case errors.As(err, &notFound):
			s.renderError(c, http.StatusNotFound, "Cryptogram not found")
		default:
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
