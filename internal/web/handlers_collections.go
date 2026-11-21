package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"

	"atria/internal/collections"
	"atria/internal/core"
)

// Collections: named lists of e-books and manuscripts with a manual or an
// automatic order. See docs/architecture/collections.md.

// sortOrderLabels names the orders in forms.
var sortOrderLabels = map[collections.SortOrder]string{
	collections.Manual:   "Manual (series, reading list)",
	collections.ByTitle:  "By title",
	collections.ByAuthor: "By author",
	collections.ByAdded:  "Recently added first",
}

type sortOrderOption struct {
	Value collections.SortOrder
	Label string
}

func sortOrderOptions() []sortOrderOption {
	options := make([]sortOrderOption, 0, len(collections.SortOrders))
	for _, o := range collections.SortOrders {
		options = append(options, sortOrderOption{o, sortOrderLabels[o]})
	}
	return options
}

func (s *Server) handleCollectionList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	list, err := collections.List(c.Request.Context(), s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load collections", err)
		return
	}
	s.render(c, "collection_list.html", gin.H{"Collections": list, "SortOrders": sortOrderOptions()})
}

func (s *Server) handleCollectionDetail(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, ok := s.loadCollection(c, user)
	if !ok {
		return
	}
	items, err := collections.Items(c.Request.Context(), s.db, user.ID, coll.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load the collection", err)
		return
	}
	s.render(c, "collection_detail.html", gin.H{
		"Collection": coll,
		"Items":      items,
		"Manual":     coll.SortOrder == collections.Manual,
		"OrderLabel": sortOrderLabels[coll.SortOrder],
		"SortOrders": sortOrderOptions(),
	})
}

func (s *Server) handleCollectionCreate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, err := collections.Create(c.Request.Context(), s.db, user.ID, c.PostForm("title"), c.PostForm("description"),
		collections.SortOrder(c.DefaultPostForm("sort_order", string(collections.Manual))))
	if err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to create the collection", err)
		return
	}
	s.redirectTo(c, "/collections/"+coll.ID.String())
}

func (s *Server) handleCollectionUpdate(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, ok := s.loadCollection(c, user)
	if !ok {
		return
	}
	if err := collections.Update(c.Request.Context(), s.db, user.ID, coll.ID, c.PostForm("title"), c.PostForm("description"),
		collections.SortOrder(c.PostForm("sort_order"))); err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to save the collection", err)
		return
	}
	s.setFlash(c, "success", "Collection saved.")
	s.redirectTo(c, "/collections/"+coll.ID.String())
}

func (s *Server) handleCollectionDelete(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, ok := s.loadCollection(c, user)
	if !ok {
		return
	}
	if err := core.SoftDeleteEntity(c.Request.Context(), s.db, user.ID, coll.ID); err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to delete the collection", err)
		return
	}
	s.setFlash(c, "success", "Collection moved to the trash. Its e-books and manuscripts stay.")
	s.redirectTo(c, "/collections")
}

// handleCollectionAddItem adds an item from its own page. The form names an
// existing collection, or a title for a new one.
func (s *Server) handleCollectionAddItem(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	ctx := c.Request.Context()
	entityID, err := core.ParseUUID(c.PostForm("entity_id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid item")
		return
	}
	var collectionID uuid.UUID
	if title := strings.TrimSpace(c.PostForm("new_title")); title != "" {
		coll, err := collections.Create(ctx, s.db, user.ID, title, "", collections.Manual)
		if err != nil {
			s.respondError(c, http.StatusBadRequest, "Failed to create the collection", err)
			return
		}
		collectionID = coll.ID
	} else if collectionID, err = core.ParseUUID(c.PostForm("collection_id")); err != nil {
		s.renderError(c, http.StatusBadRequest, "Choose a collection or type a title for a new one")
		return
	}
	if err := collections.Add(ctx, s.db, user.ID, collectionID, entityID); err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to add to the collection", err)
		return
	}
	s.redirectTo(c, localRedirectTarget(c, "/collections/"+collectionID.String()))
}

func (s *Server) handleCollectionRemoveItem(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, itemID, ok := s.loadCollectionItem(c, user)
	if !ok {
		return
	}
	if err := collections.Remove(c.Request.Context(), s.db, user.ID, coll.ID, itemID); err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to remove from the collection", err)
		return
	}
	s.redirectTo(c, localRedirectTarget(c, "/collections/"+coll.ID.String()))
}

func (s *Server) handleCollectionMoveItem(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	coll, itemID, ok := s.loadCollectionItem(c, user)
	if !ok {
		return
	}
	step, _ := strconv.Atoi(c.PostForm("step"))
	if err := collections.Move(c.Request.Context(), s.db, user.ID, coll.ID, itemID, step); err != nil {
		s.respondError(c, http.StatusBadRequest, "Failed to move the item", err)
		return
	}
	s.redirectTo(c, "/collections/"+coll.ID.String())
}

func (s *Server) loadCollection(c *gin.Context, user *core.User) (*collections.Collection, bool) {
	id, err := core.ParseUUID(c.Param("id"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid ID")
		return nil, false
	}
	coll, err := collections.Get(c.Request.Context(), s.db, user.ID, id)
	if err != nil {
		s.renderError(c, http.StatusNotFound, "Collection not found")
		return nil, false
	}
	return coll, true
}

func (s *Server) loadCollectionItem(c *gin.Context, user *core.User) (*collections.Collection, uuid.UUID, bool) {
	coll, ok := s.loadCollection(c, user)
	if !ok {
		return nil, uuid.Nil, false
	}
	itemID, err := core.ParseUUID(c.Param("item"))
	if err != nil {
		s.renderError(c, http.StatusBadRequest, "Invalid item")
		return nil, uuid.Nil, false
	}
	return coll, itemID, true
}

// collectionSection is the data of the "collections_section" partial on the
// page of an e-book or a manuscript.
func (s *Server) collectionSection(c *gin.Context, user *core.User, entityID uuid.UUID) gin.H {
	ctx := c.Request.Context()
	memberships, _ := collections.Memberships(ctx, s.db, user.ID, []uuid.UUID{entityID})
	all, _ := collections.List(ctx, s.db, user.ID)
	in := map[uuid.UUID]bool{}
	for _, m := range memberships[entityID] {
		in[m.CollectionID] = true
	}
	var others []collections.Collection
	for _, coll := range all {
		if !in[coll.ID] {
			others = append(others, coll)
		}
	}
	return gin.H{
		"EntityID":    entityID.String(),
		"Memberships": memberships[entityID],
		"Others":      others,
		"CSRFToken":   s.GetCSRFToken(c),
	}
}
