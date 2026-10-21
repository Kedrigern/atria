package web

import (
	"atria/internal/core"
	"fmt"
	"html"
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
		s.respondError(c, http.StatusInternalServerError, "Could not load the tags", err)
		return
	}

	s.render(c, "settings_tags.html", gin.H{
		"Tags":        tags,
		"SettingsTab": "tags",
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
		s.respondError(c, http.StatusInternalServerError, "Could not load the items with this tag", err)
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
		s.respondError(c, http.StatusBadRequest, "Failed to create tag", err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Refresh", "true")
		c.Status(http.StatusOK)
		return
	}

	s.setFlash(c, "success", "Tag created.")
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
		s.renderError(c, http.StatusBadRequest, "Invalid item ID")
		return
	}
	if !s.requireOwnedEntity(c, user, entityID) {
		return
	}

	tagName := c.PostForm("tag_name")
	err = core.AttachTagByTitle(c.Request.Context(), s.db, user.ID, entityID, tagName)
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Could not add the tag", err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		tags, err := core.GetEntityTags(c.Request.Context(), s.db, user.ID, entityID)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Could not load the tags", err)
			return
		}

		s.renderSnippet(c, "tags_section", gin.H{
			"ID":   entityID,
			"Tags": tags,
		})
		return
	}

	c.Redirect(http.StatusSeeOther, localRedirectTarget(c, "/"))
}

func (s *Server) handleTagAttachUniversal(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityIdentifier := c.PostForm("entity_identifier")
	tagName := c.PostForm("tag_name")

	if entityIdentifier == "" || tagName == "" {
		s.setFlash(c, "error", "Enter an item and a tag.")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	entities, err := core.FindEntities(c.Request.Context(), s.db, user.ID, "", entityIdentifier, false)
	if err != nil {
		s.flashError(c, "Database error during the search", err)
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	if len(entities) == 0 {
		s.setFlash(c, "error", "No item matches '"+entityIdentifier+"'.")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	if len(entities) > 1 {
		s.setFlash(c, "error", "More than one item matches. Use the short ID.")
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	targetEntity := entities[0]

	err = core.AttachTagByTitle(c.Request.Context(), s.db, user.ID, targetEntity.ID, tagName)
	if err != nil {
		s.flashError(c, "Could not add the tag", err)
		c.Redirect(http.StatusSeeOther, "/settings/tags")
		return
	}

	s.setFlash(c, "success", "Tag #"+tagName+" added to: "+targetEntity.Title)
	c.Redirect(http.StatusSeeOther, "/settings/tags")
}

func (s *Server) handleTagOptions(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	tags, err := core.ListTags(c.Request.Context(), s.db, user.ID)
	if err != nil {
		c.String(http.StatusOK, "")
		return
	}

	for _, t := range tags {
		if _, err := fmt.Fprintf(c.Writer, `<option value="%s">`, html.EscapeString(t.Name)); err != nil {
			return
		}
	}
}
