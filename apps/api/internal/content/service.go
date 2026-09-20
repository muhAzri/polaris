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
	"polaris-api/internal/rbac"
	"polaris-api/internal/storage"
)

const downloadURLExpiry = 15 * time.Minute

type Service struct {
	pool    *pgxpool.Pool
	modules *coursemodule.Service
	events  *eventbus.Dispatcher
	storage storage.Storage
	rbac    *rbac.Service
}

func NewService(pool *pgxpool.Pool, modules *coursemodule.Service, events *eventbus.Dispatcher, store storage.Storage, rbacService *rbac.Service) *Service {
	s := &Service{pool: pool, modules: modules, events: events, storage: store, rbac: rbacService}
	s.registerDeleters()
	return s
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
func (s *Service) AddFolderFile(ctx context.Context, moduleID, dirPath, fileName, contentType string, size int64, body io.Reader) (*FolderFile, error) {
	dirPath = normalizeDir(dirPath)
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
		INSERT INTO mod_folder_files (folder_id, file_key, file_name, file_size, content_type, position, dir_path)
		VALUES ($1, $2, $3, $4, $5, (SELECT COALESCE(MAX(position) + 1, 0) FROM mod_folder_files WHERE folder_id = $1), $6)
		RETURNING id, folder_id, file_key, file_name, file_size, content_type, position, dir_path, created_at
	`, mod.InstanceID, key, fileName, size, contentType, dirPath).Scan(&ff.ID, &ff.FolderID, &ff.FileKey, &ff.FileName, &ff.FileSize, &ff.ContentType, &ff.Position, &ff.DirPath, &ff.CreatedAt)
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
func (s *Service) AddBookChapter(ctx context.Context, moduleID, title, content string, subchapter bool) (*BookChapter, error) {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	if mod.ModuleType != "book" {
		return nil, fmt.Errorf("module %s is not a book", moduleID)
	}

	var ch BookChapter
	err = s.pool.QueryRow(ctx, `
		INSERT INTO mod_book_chapters (book_id, title, content, position, subchapter)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM mod_book_chapters WHERE book_id = $1),
			$4 AND EXISTS (SELECT 1 FROM mod_book_chapters WHERE book_id = $1))
		RETURNING id, book_id, title, content, position, subchapter, hidden, created_at
	`, mod.InstanceID, title, content, subchapter).Scan(&ch.ID, &ch.BookID, &ch.Title, &ch.Content, &ch.Position, &ch.Subchapter, &ch.Hidden, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

// --- Course content (read) ---

// CourseContent resolves every section of a course to its modules, and every
// module to its type-specific data, in one call — the course page needs
// exactly this, in position order, without the client fanning out to 6
// different per-type endpoints. Viewers without mod:viewhidden do not see
// hidden sections, modules or book chapters, and get a stub instead of the
// data for modules outside their availability window.
func (s *Service) CourseContent(ctx context.Context, courseID, userID string) ([]SectionContent, error) {
	courseContextID, err := s.rbac.ContextID(ctx, rbac.ContextLevelCourse, courseID)
	if err != nil {
		return nil, err
	}
	seeHidden, err := s.rbac.Can(ctx, userID, "mod:viewhidden", courseContextID)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, summary, position, visible FROM course_sections
		WHERE course_id = $1 AND ($2 OR visible) ORDER BY position ASC
	`, courseID, seeHidden)
	if err != nil {
		return nil, err
	}

	var sections []SectionContent
	for rows.Next() {
		var sc SectionContent
		if err := rows.Scan(&sc.ID, &sc.Title, &sc.Summary, &sc.Position, &sc.Visible); err != nil {
			rows.Close()
			return nil, err
		}
		sections = append(sections, sc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now()
	for i := range sections {
		mods, err := s.modules.ListBySection(ctx, sections[i].ID)
		if err != nil {
			return nil, err
		}

		sections[i].Modules = make([]ModuleContent, 0, len(mods))
		for _, m := range mods {
			if !m.Visible && !seeHidden {
				continue
			}
			mc := ModuleContent{
				ID: m.ID, ModuleType: m.ModuleType, Position: m.Position, Visible: m.Visible,
				Intro: m.Intro, GroupMode: m.GroupMode, GroupingID: m.GroupingID,
				AvailableFrom: m.AvailableFrom, AvailableUntil: m.AvailableUntil,
			}
			closed := (m.AvailableFrom != nil && now.Before(*m.AvailableFrom)) ||
				(m.AvailableUntil != nil && now.After(*m.AvailableUntil))
			if closed && !seeHidden {
				mc.Restricted = true
			} else {
				data, err := s.resolveModuleData(ctx, m, seeHidden)
				if err != nil {
					return nil, err
				}
				mc.Data = data
			}
			sections[i].Modules = append(sections[i].Modules, mc)
		}
	}
	if sections == nil {
		sections = []SectionContent{}
	}
	return sections, nil
}

func (s *Service) resolveModuleData(ctx context.Context, m coursemodule.Module, seeHidden bool) (any, error) {
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
			SELECT id, dir_path, file_key, file_name, file_size, content_type FROM mod_folder_files
			WHERE folder_id = $1 ORDER BY dir_path ASC, position ASC
		`, m.InstanceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		files := []FolderFileData{}
		for rows.Next() {
			var id, dirPath, fileKey, fileName, contentType string
			var size int64
			if err := rows.Scan(&id, &dirPath, &fileKey, &fileName, &size, &contentType); err != nil {
				return nil, err
			}
			downloadURL, err := s.storage.PresignGet(ctx, fileKey, downloadURLExpiry)
			if err != nil {
				return nil, err
			}
			files = append(files, FolderFileData{ID: id, DirPath: dirPath, FileName: fileName, FileSize: size, ContentType: contentType, DownloadURL: downloadURL})
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
			SELECT id, title, content, position, subchapter, hidden FROM mod_book_chapters
			WHERE book_id = $1 AND ($2 OR NOT hidden) ORDER BY position ASC
		`, m.InstanceID, seeHidden)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		chapters := []BookChapterData{}
		for rows.Next() {
			var c BookChapterData
			if err := rows.Scan(&c.ID, &c.Title, &c.Content, &c.Position, &c.Subchapter, &c.Hidden); err != nil {
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
