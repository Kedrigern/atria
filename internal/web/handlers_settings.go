package web

import (
	"atria/internal/core"
	"atria/internal/storage"
	"atria/internal/users"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleUserList(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}

	if user.Role != core.RoleAdmin {
		s.setFlash(c, "error", "Access denied.")
		c.Redirect(http.StatusSeeOther, "/settings/rss")
		return
	}

	usersList, err := users.ListUsers(c.Request.Context(), s.db)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to list users", err)
		return
	}

	s.render(c, "settings_users.html", gin.H{
		"Users":       usersList,
		"CurrentUser": user,
		"SettingsTab": "users",
	})
}

func (s *Server) handleUserCreate(c *gin.Context) {
	admin := s.getUser(c)
	if admin == nil {
		return
	}
	if admin.Role != core.RoleAdmin {
		s.setFlash(c, "error", "Access denied.")
		c.Redirect(http.StatusSeeOther, "/settings/rss")
		return
	}

	email := c.PostForm("email")
	displayName := c.PostForm("display_name")
	password := c.PostForm("password")
	role := core.Role(c.PostForm("role"))
	if role != core.RoleAdmin {
		role = core.RoleUser
	}

	if email == "" || displayName == "" || password == "" {
		s.setFlash(c, "error", "All fields are required.")
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	_, err := users.CreateUser(c.Request.Context(), s.db, email, displayName, password, role)
	if err != nil {
		s.flashError(c, "Failed to create user", err)
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	s.setFlash(c, "success", "User \""+displayName+"\" created.")
	c.Redirect(http.StatusSeeOther, "/settings/users")
}

func (s *Server) handleUserRoleUpdate(c *gin.Context) {
	admin := s.getUser(c)
	if admin == nil {
		return
	}
	if admin.Role != core.RoleAdmin {
		s.setFlash(c, "error", "Access denied.")
		c.Redirect(http.StatusSeeOther, "/settings/rss")
		return
	}

	targetEmail := c.PostForm("email")
	newRole := core.Role(c.PostForm("role"))
	if newRole != core.RoleAdmin {
		newRole = core.RoleUser
	}

	if targetEmail == admin.Email {
		s.setFlash(c, "error", "You cannot change your own role.")
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	if err := users.UpdateUserRole(c.Request.Context(), s.db, targetEmail, newRole); err != nil {
		s.flashError(c, "Failed to update role", err)
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	s.setFlash(c, "success", "Role updated.")
	c.Redirect(http.StatusSeeOther, "/settings/users")
}

func (s *Server) handleUserDelete(c *gin.Context) {
	admin := s.getUser(c)
	if admin == nil {
		return
	}
	if admin.Role != core.RoleAdmin {
		s.setFlash(c, "error", "Access denied.")
		c.Redirect(http.StatusSeeOther, "/settings/rss")
		return
	}

	targetEmail := c.PostForm("email")
	if targetEmail == admin.Email {
		s.setFlash(c, "error", "You cannot delete your own account.")
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	if err := users.DeleteUser(c.Request.Context(), s.db, targetEmail); err != nil {
		s.flashError(c, "Failed to delete user", err)
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	s.setFlash(c, "success", "User deleted.")
	c.Redirect(http.StatusSeeOther, "/settings/users")
}

// handleStorage shows how much space the user's data takes; admins also see
// a per-user overview and the database size.
func (s *Server) handleStorage(c *gin.Context) {
	user := s.getUser(c)
	if user == nil {
		return
	}
	ctx := c.Request.Context()

	usage, err := storage.ForUser(ctx, s.db, user.ID)
	if err != nil {
		s.respondError(c, http.StatusInternalServerError, "Failed to load storage usage", err)
		return
	}

	data := gin.H{
		"SettingsTab": "storage",
		"Usage":       usage,
		"IsAdmin":     user.Role == core.RoleAdmin,
	}

	if user.Role == core.RoleAdmin {
		all, err := storage.ForAllUsers(ctx, s.db)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Failed to load storage usage", err)
			return
		}
		var total storage.Usage
		for _, u := range all {
			total.Add(u)
		}
		dbSize, err := storage.DatabaseSize(ctx, s.db)
		if err != nil {
			s.respondError(c, http.StatusInternalServerError, "Failed to load database size", err)
			return
		}
		data["AllUsage"] = all
		data["TotalUsage"] = total
		data["DatabaseSize"] = dbSize
	}

	s.render(c, "settings_storage.html", data)
}
