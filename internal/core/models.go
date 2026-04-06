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

// USER

type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	DisplayName  string    `json:"display_name" db:"display_name"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"` // JSON tag "-" guard that password is never showed
	Role         Role      `json:"role" db:"role"`
	Preferences  UserPreferences
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// UserPreferences
type UserPreferences struct {
	Theme            string                 `json:"theme"`
	PaginationSize   int                    `json:"pagination_size"`
	RSSInlineDetails bool                   `json:"rss_inline_details"`
	ArticleImages    string                 `json:"article_images"`
	DomainOverrides  map[string]DomainPrefs `json:"domain_overrides"`
	DefaultDashboard *uuid.UUID             `json:"default_dashboard_id,omitempty"`
}

// DefaultPreferences funguje jako váš "default.json"
func DefaultPreferences() UserPreferences {
	return UserPreferences{
		Theme:            "system",  // Výchozí téma podle OS
		PaginationSize:   30,        // Výchozí stránkování
		RSSInlineDetails: true,      // Zobrazovat detaily u RSS
		ArticleImages:    "replace", // Naše vylepšené stahování obrázků
		DomainOverrides:  make(map[string]DomainPrefs),
	}
}

// ENTITY

// Entity represents the core polymorphic record in Atria.
type Entity struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty" db:"parent_id"` // Pointer, because it can be NULL (root)
	OwnerID    uuid.UUID  `json:"owner_id" db:"owner_id"`
	Type       EntityType `json:"type" db:"type"`
	Visibility Visibility `json:"visibility" db:"visibility"`
	Title      string     `json:"title" db:"title"`
	Slug       string     `json:"slug" db:"slug"`
	Path       string     `json:"path"`

	// Pessimistic Locking
	LockedBy *uuid.UUID `json:"locked_by,omitempty" db:"locked_by"`
	LockedAt *time.Time `json:"locked_at,omitempty" db:"locked_at"`

	UpdatedBy *uuid.UUID `json:"updated_by,omitempty" db:"updated_by"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" db:"deleted_at"` // Soft delete
}

// ARTICLE

type Article struct {
	ID          uuid.UUID `json:"id" db:"id"` // Always Entity.ID
	OriginalURL string    `json:"original_url" db:"original_url"`
	Domain      string    `json:"domain" db:"domain"`
	IsArchived  bool      `json:"is_archived" db:"is_archived"`
	UserNote    *string   `json:"user_note,omitempty" db:"user_note"`
	HTMLContent *string   `json:"html_content,omitempty" db:"html_content"`
	TextContent *string   `json:"text_content,omitempty" db:"text_content"`
}

// Article DomainPrefs solves specific settings for domain
type DomainPrefs struct {
	ArticleImages string `json:"article_images"` // "replace", "strip", "link"
}

// RSS

type RSSFilterRules struct {
	IncludeKeywords []string `json:"include_keywords,omitempty"`
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"`
}

type RSSFeed struct {
	ID                 uuid.UUID `json:"id" db:"id"`
	FeedURL            string    `json:"feed_url" db:"feed_url"`
	SiteURL            *string   `json:"site_url,omitempty" db:"site_url"`
	EtagHeader         *string   `json:"etag_header,omitempty" db:"etag_header"`
	LastModifiedHeader *string   `json:"last_modified_header,omitempty" db:"last_modified_header"`
	NextFetchAt        time.Time `json:"next_fetch_at" db:"next_fetch_at"`

	// Add these diagnostic fields to match the V1 schema
	LastFetchedAt   *time.Time `json:"last_fetched_at,omitempty" db:"last_fetched_at"`
	LastFetchStatus *int       `json:"last_fetch_status,omitempty" db:"last_fetch_status"`
	LastFetchError  *string    `json:"last_fetch_error,omitempty" db:"last_fetch_error"`

	HTTPAuthType     *string        `json:"http_auth_type,omitempty" db:"http_auth_type"`
	HTTPAuthUsername *string        `json:"http_auth_username,omitempty" db:"http_auth_username"`
	HTTPAuthToken    *string        `json:"http_auth_token,omitempty" db:"http_auth_token"`
	FilterRules      RSSFilterRules `json:"filter_rules"`
}

type FeedSummary struct {
	ID              uuid.UUID
	Title           string
	FeedURL         string
	SiteURL         *string
	LastFetchedAt   *time.Time
	LastFetchStatus *int
	LastFetchError  *string
}

// RSSItem represents a single entry from a feed for triage.
type RSSItem struct {
	ID          uuid.UUID `json:"id"`
	FeedID      uuid.UUID `json:"feed_id"`
	SourceName  string    `json:"source_name"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// NOTE

type Note struct {
	ID              uuid.UUID `json:"id" db:"id"`
	Icon            *string   `json:"icon,omitempty" db:"icon"`
	MarkdownContent string    `json:"markdown_content" db:"markdown_content"`
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
