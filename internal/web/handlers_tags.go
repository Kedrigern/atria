package web

import (
	"atria/internal/core"
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleTags list all tags of the user
func (s *Server) handleTags(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	tags, err := core.ListTags(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Chyba při načítání tagů: "+err.Error())
		return
	}

	s.render(c, "tag_list.html", gin.H{
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

func (s *Server) handleTagAdd(c *gin.Context) {
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

	s.handleSuccess(c, "/tags", "Tag vytvořen.")
}

func (s *Server) handleTagAttach(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	entityID, ok := s.getUUIDParam(c, "id")
	if !ok {
		return
	}

	tagName := c.PostForm("tag_name")
	err := core.AttachTagByTitle(c.Request.Context(), s.db, user.ID, entityID, tagName)
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Nepodařilo se připojit tag: "+err.Error())
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
			`<a href="/tags/`+tagName+`" class="tag">#`+tagName+`</a>`,
		))
		return
	}

	c.Redirect(http.StatusSeeOther, c.Request.Referer())
}
