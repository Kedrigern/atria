package web

import (
	"atria/internal/core"
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
		c.Redirect(http.StatusSeeOther, "/settings")
		return
	}

	usersList, err := users.ListUsers(c.Request.Context(), s.db)
	if err != nil {
		s.renderError(c, http.StatusInternalServerError, "Failed to list users: "+err.Error())
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
		c.Redirect(http.StatusSeeOther, "/settings")
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
		s.setFlash(c, "error", "Failed to create user: "+err.Error())
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
		c.Redirect(http.StatusSeeOther, "/settings")
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
		s.setFlash(c, "error", "Failed to update role: "+err.Error())
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
		c.Redirect(http.StatusSeeOther, "/settings")
		return
	}

	targetEmail := c.PostForm("email")
	if targetEmail == admin.Email {
		s.setFlash(c, "error", "You cannot delete your own account.")
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	if err := users.DeleteUser(c.Request.Context(), s.db, targetEmail); err != nil {
		s.setFlash(c, "error", "Failed to delete user: "+err.Error())
		c.Redirect(http.StatusSeeOther, "/settings/users")
		return
	}

	s.setFlash(c, "success", "User deleted.")
	c.Redirect(http.StatusSeeOther, "/settings/users")
}
