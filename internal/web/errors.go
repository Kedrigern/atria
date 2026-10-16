package web

import (
	"atria/internal/core"
	"errors"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// userFacingError builds the message shown to the user for a failed action.
// message is the handler's own description ("Failed to save note"). If err
// carries a core.UserError or a recognised constraint violation, its text is
// appended; any other detail (SQL, file paths, template internals) is only
// logged.
func userFacingError(message string, err error) string {
	if detail, ok := core.UserMessage(err); ok {
		return message + ": " + detail
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23505": // unique_violation
			return message + ": an item with this name already exists"
		case "23514", "23502": // check_violation, not_null_violation
			return message + ": invalid value"
		case "22001": // string_data_right_truncation
			return message + ": value is too long"
		}
	}
	return message
}

func logRequestError(c *gin.Context, message string, err error) {
	log.Printf("ERROR %s %s: %s: %v", c.Request.Method, c.Request.URL.Path, message, err)
}

// respondError logs err with request context and responds with a safe message.
func (s *Server) respondError(c *gin.Context, status int, message string, err error) {
	logRequestError(c, message, err)
	s.renderError(c, status, userFacingError(message, err))
}

// flashError logs err and stores a safe message as an error flash.
func (s *Server) flashError(c *gin.Context, message string, err error) {
	logRequestError(c, message, err)
	s.setFlash(c, "error", userFacingError(message, err))
}
