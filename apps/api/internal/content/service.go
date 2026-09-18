// Package content implements six static content modules — mod/label,
// mod/page, mod/url, mod/resource, mod/folder, mod/book. Each type owns
// its own instance table and attaches itself to course_modules via
// coursemodule.Service, rather than course/section code needing to know
// about every module type that exists.
package content

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/coursemodule"
	"polaris-api/internal/eventbus"
	"polaris-api/internal/storage"
)

const downloadURLExpiry = 15 * time.Minute

type Service struct {
	pool    *pgxpool.Pool
	modules *coursemodule.Service
	events  *eventbus.Dispatcher
	storage storage.Storage
}

func NewService(pool *pgxpool.Pool, modules *coursemodule.Service, events *eventbus.Dispatcher, store storage.Storage) *Service {
	return &Service{pool: pool, modules: modules, events: events, storage: store}
}

func (s *Service) emitCreated(ctx context.Context, moduleType, moduleID, sectionID, userID string) {
	s.events.Dispatch(ctx, eventbus.Event{
		Name:   "mod.created",
		UserID: &userID,
		Data:   map[string]any{"module_type": moduleType, "module_id": moduleID, "section_id": sectionID},
	})
}

func (s *Service) CourseIDForSection(ctx context.Context, sectionID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT course_id FROM course_sections WHERE id = $1`, sectionID).Scan(&id)
	return id, err
}

func (s *Service) CourseIDForModule(ctx context.Context, moduleID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT cs.course_id FROM course_modules cm
		JOIN course_sections cs ON cs.id = cm.section_id
		WHERE cm.id = $1
	`, moduleID).Scan(&id)
	return id, err
}

// --- Label ---

func (s *Service) CreateLabel(ctx context.Context, sectionID, userID, content string) (*coursemodule.Module, *Label, error) {
	var l Label
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_labels (content) VALUES ($1) RETURNING id, content, created_at
	`, content).Scan(&l.ID, &l.Content, &l.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "label", l.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "label", m.ID, sectionID, userID)
	return m, &l, nil
}

// --- Page ---

func (s *Service) CreatePage(ctx context.Context, sectionID, userID, title, content string) (*coursemodule.Module, *Page, error) {
	var p Page
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_pages (title, content) VALUES ($1, $2) RETURNING id, title, content, created_at
	`, title, content).Scan(&p.ID, &p.Title, &p.Content, &p.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "page", p.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "page", m.ID, sectionID, userID)
	return m, &p, nil
}

// --- URL ---

func (s *Service) CreateURL(ctx context.Context, sectionID, userID, title, url, description string) (*coursemodule.Module, *URL, error) {
	var u URL
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_urls (title, url, description) VALUES ($1, $2, $3) RETURNING id, title, url, description, created_at
	`, title, url, description).Scan(&u.ID, &u.Title, &u.URL, &u.Description, &u.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "url", u.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "url", m.ID, sectionID, userID)
	return m, &u, nil
}

// --- Resource ---

func (s *Service) CreateResource(ctx context.Context, sectionID, userID, title, fileName, contentType string, size int64, body io.Reader) (*coursemodule.Module, *Resource, error) {
	key := fmt.Sprintf("resources/%s/%s", uuid.NewString(), fileName)
	if err := s.storage.Put(ctx, key, body, size, contentType); err != nil {
		return nil, nil, err
	}

	var res Resource
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_resources (title, file_key, file_name, file_size, content_type)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, title, file_key, file_name, file_size, content_type, created_at
	`, title, key, fileName, size, contentType).Scan(&res.ID, &res.Title, &res.FileKey, &res.FileName, &res.FileSize, &res.ContentType, &res.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "resource", res.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "resource", m.ID, sectionID, userID)
	return m, &res, nil
}

// --- Folder ---

func (s *Service) CreateFolder(ctx context.Context, sectionID, userID, title, description string) (*coursemodule.Module, *Folder, error) {
	var f Folder
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_folders (title, description) VALUES ($1, $2) RETURNING id, title, description, created_at
	`, title, description).Scan(&f.ID, &f.Title, &f.Description, &f.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "folder", f.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "folder", m.ID, sectionID, userID)
	return m, &f, nil
}

// AddFolderFile takes a course_modules id (not the folder row's own id) so
// handlers only ever deal in module ids — the same id used for capability
// checks — instead of juggling two different ids for one resource.
func (s *Service) AddFolderFile(ctx context.Context, moduleID, fileName, contentType string, size int64, body io.Reader) (*FolderFile, error) {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	if mod.ModuleType != "folder" {
		return nil, fmt.Errorf("module %s is not a folder", moduleID)
	}

	key := fmt.Sprintf("folders/%s/%s", uuid.NewString(), fileName)
	if err := s.storage.Put(ctx, key, body, size, contentType); err != nil {
		return nil, err
	}

	var ff FolderFile
	err = s.pool.QueryRow(ctx, `
		INSERT INTO mod_folder_files (folder_id, file_key, file_name, file_size, content_type, position)
		VALUES ($1, $2, $3, $4, $5, (SELECT COALESCE(MAX(position) + 1, 0) FROM mod_folder_files WHERE folder_id = $1))
		RETURNING id, folder_id, file_key, file_name, file_size, content_type, position, created_at
	`, mod.InstanceID, key, fileName, size, contentType).Scan(&ff.ID, &ff.FolderID, &ff.FileKey, &ff.FileName, &ff.FileSize, &ff.ContentType, &ff.Position, &ff.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ff, nil
}

// --- Book ---

func (s *Service) CreateBook(ctx context.Context, sectionID, userID, title, intro string) (*coursemodule.Module, *Book, error) {
	var b Book
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mod_books (title, intro) VALUES ($1, $2) RETURNING id, title, intro, created_at
	`, title, intro).Scan(&b.ID, &b.Title, &b.Intro, &b.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	m, err := s.modules.Attach(ctx, sectionID, "book", b.ID)
	if err != nil {
		return nil, nil, err
	}
	s.emitCreated(ctx, "book", m.ID, sectionID, userID)
	return m, &b, nil
}

// AddBookChapter takes a course_modules id for the same reason AddFolderFile
// does — see above.
func (s *Service) AddBookChapter(ctx context.Context, moduleID, title, content string) (*BookChapter, error) {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	if mod.ModuleType != "book" {
		return nil, fmt.Errorf("module %s is not a book", moduleID)
	}

	var ch BookChapter
	err = s.pool.QueryRow(ctx, `
		INSERT INTO mod_book_chapters (book_id, title, content, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM mod_book_chapters WHERE book_id = $1))
		RETURNING id, book_id, title, content, position, created_at
	`, mod.InstanceID, title, content).Scan(&ch.ID, &ch.BookID, &ch.Title, &ch.Content, &ch.Position, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// --- Course content (read) ---

// CourseContent resolves every section of a course to its modules, and every
// module to its type-specific data, in one call — the course page needs
// exactly this, in position order, without the client fanning out to 6
// different per-type endpoints.
func (s *Service) CourseContent(ctx context.Context, courseID string) ([]SectionContent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, position FROM course_sections WHERE course_id = $1 ORDER BY position ASC
	`, courseID)
	if err != nil {
		return nil, err
	}

	type sectionRow struct {
		id       string
		title    string
		position int
	}
	var sectionRows []sectionRow
	for rows.Next() {
		var sr sectionRow
		if err := rows.Scan(&sr.id, &sr.title, &sr.position); err != nil {
			rows.Close()
			return nil, err
		}
		sectionRows = append(sectionRows, sr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sections := make([]SectionContent, 0, len(sectionRows))
	for _, sr := range sectionRows {
		mods, err := s.modules.ListBySection(ctx, sr.id)
		if err != nil {
			return nil, err
		}

		moduleContents := make([]ModuleContent, 0, len(mods))
		for _, m := range mods {
			data, err := s.resolveModuleData(ctx, m)
			if err != nil {
				return nil, err
			}
			moduleContents = append(moduleContents, ModuleContent{
				ID: m.ID, ModuleType: m.ModuleType, Position: m.Position, Visible: m.Visible, Data: data,
			})
		}

		sections = append(sections, SectionContent{
			ID: sr.id, Title: sr.title, Position: sr.position, Modules: moduleContents,
		})
	}
	return sections, nil
}

func (s *Service) resolveModuleData(ctx context.Context, m coursemodule.Module) (any, error) {
	switch m.ModuleType {
	case "label":
		var l LabelData
		if err := s.pool.QueryRow(ctx, `SELECT content FROM mod_labels WHERE id = $1`, m.InstanceID).Scan(&l.Content); err != nil {
			return nil, err
		}
		return l, nil

	case "page":
		var p PageData
		if err := s.pool.QueryRow(ctx, `SELECT title, content FROM mod_pages WHERE id = $1`, m.InstanceID).Scan(&p.Title, &p.Content); err != nil {
			return nil, err
		}
		return p, nil

	case "url":
		var u URLData
		if err := s.pool.QueryRow(ctx, `SELECT title, url, description FROM mod_urls WHERE id = $1`, m.InstanceID).Scan(&u.Title, &u.URL, &u.Description); err != nil {
			return nil, err
		}
		return u, nil

	case "resource":
		var title, fileName, fileKey, contentType string
		var size int64
		if err := s.pool.QueryRow(ctx, `
			SELECT title, file_key, file_name, file_size, content_type FROM mod_resources WHERE id = $1
		`, m.InstanceID).Scan(&title, &fileKey, &fileName, &size, &contentType); err != nil {
			return nil, err
		}
		downloadURL, err := s.storage.PresignGet(ctx, fileKey, downloadURLExpiry)
		if err != nil {
			return nil, err
		}
		return ResourceData{Title: title, FileName: fileName, FileSize: size, ContentType: contentType, DownloadURL: downloadURL}, nil

	case "folder":
		var title, description string
		if err := s.pool.QueryRow(ctx, `SELECT title, description FROM mod_folders WHERE id = $1`, m.InstanceID).Scan(&title, &description); err != nil {
			return nil, err
		}

		rows, err := s.pool.Query(ctx, `
			SELECT file_key, file_name, file_size, content_type FROM mod_folder_files
			WHERE folder_id = $1 ORDER BY position ASC
		`, m.InstanceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		files := []FolderFileData{}
		for rows.Next() {
			var fileKey, fileName, contentType string
			var size int64
			if err := rows.Scan(&fileKey, &fileName, &size, &contentType); err != nil {
				return nil, err
			}
			downloadURL, err := s.storage.PresignGet(ctx, fileKey, downloadURLExpiry)
			if err != nil {
				return nil, err
			}
			files = append(files, FolderFileData{FileName: fileName, FileSize: size, ContentType: contentType, DownloadURL: downloadURL})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return FolderData{Title: title, Description: description, Files: files}, nil

	case "book":
		var title, intro string
		if err := s.pool.QueryRow(ctx, `SELECT title, intro FROM mod_books WHERE id = $1`, m.InstanceID).Scan(&title, &intro); err != nil {
			return nil, err
		}

		rows, err := s.pool.Query(ctx, `
			SELECT id, title, content, position FROM mod_book_chapters
			WHERE book_id = $1 ORDER BY position ASC
		`, m.InstanceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		chapters := []BookChapterData{}
		for rows.Next() {
			var c BookChapterData
			if err := rows.Scan(&c.ID, &c.Title, &c.Content, &c.Position); err != nil {
				return nil, err
			}
			chapters = append(chapters, c)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return BookData{Title: title, Intro: intro, Chapters: chapters}, nil

	default:
		return nil, fmt.Errorf("unknown module type: %s", m.ModuleType)
	}
}
