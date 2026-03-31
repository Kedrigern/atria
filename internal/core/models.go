package core

import (
	"time"

	"github.com/gofrs/uuid/v5"
)

// ==========================================
// ENUMS
// ==========================================

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityUsers   Visibility = "users"
	VisibilityPublic  Visibility = "public"
)

type EntityType string

const (
	TypeNote        EntityType = "note"
	TypeArticle     EntityType = "article"
	TypeRSS         EntityType = "rss"
	TypeSpreadsheet EntityType = "spreadsheet"
	TypeFolder      EntityType = "folder"
)

// ==========================================
// DOMAIN MODELS
// ==========================================

type User struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	DisplayName  string     `json:"display_name" db:"display_name"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"` // JSON tag "-" guard that password is never showed
	Role         Role       `json:"role" db:"role"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

type Entity struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty" db:"parent_id"` // Pointer, because it can be NULL (root)
	OwnerID    uuid.UUID  `json:"owner_id" db:"owner_id"`
	Type       EntityType `json:"type" db:"type"`
	Visibility Visibility `json:"visibility" db:"visibility"`
	Title      string     `json:"title" db:"title"`
	Slug       string     `json:"slug" db:"slug"`

	// Pessimistic Locking
	LockedBy   *uuid.UUID `json:"locked_by,omitempty" db:"locked_by"`
	LockedAt   *time.Time `json:"locked_at,omitempty" db:"locked_at"`

	UpdatedBy  *uuid.UUID `json:"updated_by,omitempty" db:"updated_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty" db:"deleted_at"` // Soft delete
}

type Article struct {
	ID          uuid.UUID `json:"id" db:"id"` // Always Entity.ID
	OriginalURL string    `json:"original_url" db:"original_url"`
	Domain      string    `json:"domain" db:"domain"`
	IsArchived  bool      `json:"is_archived" db:"is_archived"`
	UserNote    *string   `json:"user_note,omitempty" db:"user_note"`
	HTMLContent *string   `json:"html_content,omitempty" db:"html_content"`
	TextContent *string   `json:"text_content,omitempty" db:"text_content"`
}

type Note struct {
	ID              uuid.UUID `json:"id" db:"id"`
	Icon            *string   `json:"icon,omitempty" db:"icon"`
	MarkdownContent string    `json:"markdown_content" db:"markdown_content"`
}

type RSSFeed struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	FeedURL            string     `json:"feed_url" db:"feed_url"`
	SiteURL            *string    `json:"site_url,omitempty" db:"site_url"`
	Description        *string    `json:"description,omitempty" db:"description"`
	EtagHeader         *string    `json:"etag_header,omitempty" db:"etag_header"`
	LastModifiedHeader *string    `json:"last_modified_header,omitempty" db:"last_modified_header"`
	ErrorCount         int        `json:"error_count" db:"error_count"`
	LastFetchedAt      *time.Time `json:"last_fetched_at,omitempty" db:"last_fetched_at"`
	NextFetchAt        time.Time  `json:"next_fetch_at" db:"next_fetch_at"`
}

type Tag struct {
	ID          uuid.UUID `json:"id" db:"id"`
	OwnerID     uuid.UUID `json:"owner_id" db:"owner_id"`
	Name        string    `json:"name" db:"name"`
	Description *string   `json:"description,omitempty" db:"description"`
	Color       *string   `json:"color,omitempty" db:"color"`
	Icon        *string   `json:"icon,omitempty" db:"icon"`
	IsSystem    bool      `json:"is_system" db:"is_system"`
}

type Attachment struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	OwnerID    uuid.UUID  `json:"owner_id" db:"owner_id"`
	Filename   string     `json:"filename" db:"filename"`
	MimeType   string     `json:"mime_type" db:"mime_type"`
	SizeBytes  int        `json:"size_bytes" db:"size_bytes"`
	FileHash   string     `json:"file_hash" db:"file_hash"`
	DiskPath   string     `json:"disk_path" db:"disk_path"`
	Visibility Visibility `json:"visibility" db:"visibility"`
	UpdatedBy  *uuid.UUID `json:"updated_by,omitempty" db:"updated_by"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
}
