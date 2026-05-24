package web

import (
	"atria/internal/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleTags list all tags of the user
func (s *Server) handleTagList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	tags, err := core.ListTags(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při načítání tagů: "+err.Error())
		return
	}

	s.render(c, "settings_tags.html", gin.H{
		"Tags": tags,
	})
}

// handleTagDetail list all entities with given tag
func (s *Server) handleTagDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	tagName := c.Param("name")

	entities, err := core.GetTagEntities(c.Request.Context(), s.db, user.ID, tagName)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při načítání entit tagu: "+err.Error())
		return
	}

	s.render(c, "tag_detail.html", gin.H{
		"TagName":  tagName,
		"Entities": entities,
	})
}

// handleTagAdd process cration of new tag
func (s *Server) handleTagCreate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	name := c.PostForm("name")

	_, err := core.CreateTag(c.Request.Context(), s.db, user.ID, name, false)
	if err != nil {
		s.renderError(c, http.StatusBadRequest, err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Tag vytvořen.")
	c.Redirect(http.StatusSeeOther, "/settings/tags")
}

// handleTagAttach add tag to specific entity (article, note)
func (s *Server) handleTagAttach(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Neplatné ID entity")
		return
	}

	tagName := c.PostForm("tag_name")
	err = core.AttachTagByTitle(c.Request.Context(), s.db, user.ID, entityID, tagName)
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Nepodařilo se připojit tag: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		s.renderSnippet(c, "tag_link", tagName)
		return
	}

	c.Redirect(http.StatusSeeOther, c.Request.Referer())
}

func (s *Server) handleTagAttachUniversal(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityIdentifier := c.PostForm("entity_identifier")
	tagName := c.PostForm("tag_name")

	if entityIdentifier == "" || tagName == "" {
		s.setFlash(c, "error", "Identifikátor entity i tag musí být vyplněny.")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	entities, err := core.FindEntities(c.Request.Context(), s.db, user.ID, "", entityIdentifier, false)
	if err != nil {
		s.setFlash(c, "error", "Chyba databáze při vyhledávání: "+err.Error())
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	if len(entities) == 0 {
		s.setFlash(c, "error", "Nenalezena žádná entita odpovídající '"+entityIdentifier+"'.")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	if len(entities) > 1 {
		s.setFlash(c, "error", "Nalezeno více entit. Buďte specifičtější (zadejte Short ID).")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	targetEntity := entities[0]

	err = core.AttachTagByTitle(c.Request.Context(), s.db, user.ID, targetEntity.ID, tagName)
	if err != nil {
		s.setFlash(c, "error", "Nepodařilo se připojit tag: "+err.Error())
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	s.setFlash(c, "success", "Tag #"+tagName+" byl úspěšně připojen k: "+targetEntity.Title)
	c.Redirect(http.StatusSeeOther, "/settings/tags")
}
