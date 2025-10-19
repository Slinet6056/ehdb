package database

import (
	"time"
)

// Gallery represents a gallery record
type Gallery struct {
	Gid          int       `json:"gid"`
	Token        string    `json:"token"`
	ArchiverKey  string    `json:"archiver_key"`
	Title        string    `json:"title"`
	TitleJpn     string    `json:"title_jpn"`
	Category     string    `json:"category"`
	Thumb        string    `json:"thumb"`
	Uploader     *string   `json:"uploader"`
	Posted       time.Time `json:"posted"`
	Filecount    int       `json:"filecount"`
	Filesize     int64     `json:"filesize"`
	Expunged     bool      `json:"expunged"`
	Removed      bool      `json:"removed"`
	Replaced     bool      `json:"replaced"`
	Rating       float64   `json:"rating"`
	Torrentcount int       `json:"torrentcount"`
	RootGid      *int      `json:"root_gid"`
	Bytorrent    bool      `json:"bytorrent"`
	Tags         []string  `json:"tags"`
	Torrents     []Torrent `json:"torrents"`
}

// Tag represents a tag record
type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Torrent represents a torrent record
type Torrent struct {
	ID       int     `json:"id"`
	Gid      int     `json:"gid"`
	Name     string  `json:"name"`
	Hash     *string `json:"hash"`
	Addedstr *string `json:"addedstr"`
	Fsizestr *string `json:"fsizestr"`
	Uploader string  `json:"uploader"`
	Expunged bool    `json:"expunged"`
}

// GalleryMetadata represents metadata from E-Hentai API
type GalleryMetadata struct {
	Gid          int      `json:"gid"`
	Token        string   `json:"token"`
	ArchiverKey  string   `json:"archiver_key"`
	Title        string   `json:"title"`
	TitleJpn     string   `json:"title_jpn"`
	Category     string   `json:"category"`
	Thumb        string   `json:"thumb"`
	Uploader     string   `json:"uploader"`
	Posted       string   `json:"posted"`
	Filecount    string   `json:"filecount"`
	Filesize     int64    `json:"filesize"`
	Expunged     bool     `json:"expunged"`
	Rating       string   `json:"rating"`
	Torrentcount string   `json:"torrentcount"`
	Tags         []string `json:"tags"`
	Error        string   `json:"error,omitempty"`
}

// APIResponse represents the standard API response format
type APIResponse struct {
	Data       interface{} `json:"data"`
	Code       int         `json:"code"`
	Message    string      `json:"message"`
	Total      *int64      `json:"total,omitempty"`
	NextCursor *string     `json:"next_cursor,omitempty"` // Unix timestamp for cursor-based pagination
}
