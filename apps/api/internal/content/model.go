package content

import "time"

type Label struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Page struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type URL struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Resource struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	FileKey     string    `json:"-"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

type Folder struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type FolderFile struct {
	ID          string    `json:"id"`
	FolderID    string    `json:"folder_id"`
	FileKey     string    `json:"-"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
}

type Book struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Intro     string    `json:"intro"`
	CreatedAt time.Time `json:"created_at"`
}

type BookChapter struct {
	ID        string    `json:"id"`
	BookID    string    `json:"book_id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// SectionContent/ModuleContent shape the read-only course-page view (GET
// .../content): sections in position order, each with its modules resolved
// to type-specific data, so the client doesn't need 6 follow-up requests
// (one per module type) just to render a course page.
type SectionContent struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Position int             `json:"position"`
	Modules  []ModuleContent `json:"modules"`
}

type ModuleContent struct {
	ID         string `json:"id"`
	ModuleType string `json:"module_type"`
	Position   int    `json:"position"`
	Visible    bool   `json:"visible"`
	Data       any    `json:"data"`
}

type LabelData struct {
	Content string `json:"content"`
}

type PageData struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type URLData struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

type ResourceData struct {
	Title       string `json:"title"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

type FolderFileData struct {
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
}

type FolderData struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Files       []FolderFileData `json:"files"`
}

type BookChapterData struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Position int    `json:"position"`
}

type BookData struct {
	Title    string            `json:"title"`
	Intro    string            `json:"intro"`
	Chapters []BookChapterData `json:"chapters"`
}
