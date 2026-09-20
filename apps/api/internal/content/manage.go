package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidContent = errors.New("invalid content for this module type")
	ErrNotFound       = errors.New("not found")
)

// registerDeleters teaches coursemodule how to remove each type's instance
// row (and stored files) when a module, section or course is deleted.
func (s *Service) registerDeleters() {
	s.modules.RegisterDeleter("label", func(ctx context.Context, id string) error {
		_, err := s.pool.Exec(ctx, `DELETE FROM mod_labels WHERE id = $1`, id)
		return err
	})
	s.modules.RegisterDeleter("page", func(ctx context.Context, id string) error {
		_, err := s.pool.Exec(ctx, `DELETE FROM mod_pages WHERE id = $1`, id)
		return err
	})
	s.modules.RegisterDeleter("url", func(ctx context.Context, id string) error {
		_, err := s.pool.Exec(ctx, `DELETE FROM mod_urls WHERE id = $1`, id)
		return err
	})
	s.modules.RegisterDeleter("resource", func(ctx context.Context, id string) error {
		var key string
		err := s.pool.QueryRow(ctx, `SELECT file_key FROM mod_resources WHERE id = $1`, id).Scan(&key)
		if err == nil {
			_ = s.storage.Delete(ctx, key)
		}
		_, err = s.pool.Exec(ctx, `DELETE FROM mod_resources WHERE id = $1`, id)
		return err
	})
	s.modules.RegisterDeleter("folder", func(ctx context.Context, id string) error {
		rows, err := s.pool.Query(ctx, `SELECT file_key FROM mod_folder_files WHERE folder_id = $1`, id)
		if err != nil {
			return err
		}
		var keys []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
		}
		rows.Close()
		for _, k := range keys {
			_ = s.storage.Delete(ctx, k)
		}
		_, err = s.pool.Exec(ctx, `DELETE FROM mod_folders WHERE id = $1`, id)
		return err
	})
	s.modules.RegisterDeleter("book", func(ctx context.Context, id string) error {
		_, err := s.pool.Exec(ctx, `DELETE FROM mod_books WHERE id = $1`, id)
		return err
	})
}

// normalizeDir turns any folder path into "/a/b" form with no trailing
// slash, rooted at "/".
func normalizeDir(p string) string {
	parts := []string{}
	for _, seg := range strings.Split(p, "/") {
		seg = strings.TrimSpace(seg)
		if seg != "" && seg != "." && seg != ".." {
			parts = append(parts, seg)
		}
	}
	return "/" + strings.Join(parts, "/")
}

// UpdateContent edits the type-specific fields of a module from a JSON body;
// only the fields present are changed.
func (s *Service) UpdateContent(ctx context.Context, moduleID string, body json.RawMessage) error {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return err
	}

	var in struct {
		Title       *string `json:"title"`
		Content     *string `json:"content"`
		URL         *string `json:"url"`
		Description *string `json:"description"`
		Intro       *string `json:"intro"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return ErrInvalidContent
	}
	if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
		return ErrInvalidContent
	}

	switch mod.ModuleType {
	case "label":
		_, err = s.pool.Exec(ctx, `UPDATE mod_labels SET content = COALESCE($2, content) WHERE id = $1`, mod.InstanceID, in.Content)
	case "page":
		_, err = s.pool.Exec(ctx, `UPDATE mod_pages SET title = COALESCE($2, title), content = COALESCE($3, content) WHERE id = $1`, mod.InstanceID, in.Title, in.Content)
	case "url":
		_, err = s.pool.Exec(ctx, `UPDATE mod_urls SET title = COALESCE($2, title), url = COALESCE($3, url), description = COALESCE($4, description) WHERE id = $1`, mod.InstanceID, in.Title, in.URL, in.Description)
	case "resource":
		_, err = s.pool.Exec(ctx, `UPDATE mod_resources SET title = COALESCE($2, title) WHERE id = $1`, mod.InstanceID, in.Title)
	case "folder":
		_, err = s.pool.Exec(ctx, `UPDATE mod_folders SET title = COALESCE($2, title), description = COALESCE($3, description) WHERE id = $1`, mod.InstanceID, in.Title, in.Description)
	case "book":
		_, err = s.pool.Exec(ctx, `UPDATE mod_books SET title = COALESCE($2, title), intro = COALESCE($3, intro) WHERE id = $1`, mod.InstanceID, in.Title, in.Intro)
	default:
		return fmt.Errorf("unknown module type: %s", mod.ModuleType)
	}
	return err
}

type FolderFileUpdate struct {
	FileName *string `json:"file_name"`
	DirPath  *string `json:"dir_path"`
}

func (s *Service) folderIDFor(ctx context.Context, moduleID string) (string, error) {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return "", err
	}
	if mod.ModuleType != "folder" {
		return "", fmt.Errorf("module %s is not a folder", moduleID)
	}
	return mod.InstanceID, nil
}

// UpdateFolderFile renames a file or moves it into another subfolder.
func (s *Service) UpdateFolderFile(ctx context.Context, moduleID, fileID string, u FolderFileUpdate) error {
	folderID, err := s.folderIDFor(ctx, moduleID)
	if err != nil {
		return err
	}
	var dir *string
	if u.DirPath != nil {
		d := normalizeDir(*u.DirPath)
		dir = &d
	}
	if u.FileName != nil && strings.TrimSpace(*u.FileName) == "" {
		return ErrInvalidContent
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE mod_folder_files SET file_name = COALESCE($3, file_name), dir_path = COALESCE($4, dir_path)
		WHERE id = $1 AND folder_id = $2
	`, fileID, folderID, u.FileName, dir)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) DeleteFolderFile(ctx context.Context, moduleID, fileID string) error {
	folderID, err := s.folderIDFor(ctx, moduleID)
	if err != nil {
		return err
	}
	var key string
	err = s.pool.QueryRow(ctx, `DELETE FROM mod_folder_files WHERE id = $1 AND folder_id = $2 RETURNING file_key`, fileID, folderID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_ = s.storage.Delete(ctx, key)
	return nil
}

type ChapterUpdate struct {
	Title      *string `json:"title"`
	Content    *string `json:"content"`
	Subchapter *bool   `json:"subchapter"`
	Hidden     *bool   `json:"hidden"`
	Position   *int    `json:"position"`
}

func (s *Service) bookIDFor(ctx context.Context, moduleID string) (string, error) {
	mod, err := s.modules.Get(ctx, moduleID)
	if err != nil {
		return "", err
	}
	if mod.ModuleType != "book" {
		return "", fmt.Errorf("module %s is not a book", moduleID)
	}
	return mod.InstanceID, nil
}

// UpdateBookChapter edits a chapter and, when a position is given, moves it.
// The first chapter is always a main chapter.
func (s *Service) UpdateBookChapter(ctx context.Context, moduleID, chapterID string, u ChapterUpdate) (*BookChapter, error) {
	bookID, err := s.bookIDFor(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	if u.Title != nil && strings.TrimSpace(*u.Title) == "" {
		return nil, ErrInvalidContent
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var oldPos int
	err = tx.QueryRow(ctx, `SELECT position FROM mod_book_chapters WHERE id = $1 AND book_id = $2 FOR UPDATE`, chapterID, bookID).Scan(&oldPos)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if u.Position != nil && *u.Position != oldPos {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM mod_book_chapters WHERE book_id = $1`, bookID).Scan(&count); err != nil {
			return nil, err
		}
		newPos := *u.Position
		if newPos < 0 || newPos >= count {
			return nil, ErrInvalidContent
		}
		if newPos > oldPos {
			_, err = tx.Exec(ctx, `UPDATE mod_book_chapters SET position = position - 1 WHERE book_id = $1 AND position > $2 AND position <= $3`, bookID, oldPos, newPos)
		} else {
			_, err = tx.Exec(ctx, `UPDATE mod_book_chapters SET position = position + 1 WHERE book_id = $1 AND position >= $3 AND position < $2`, bookID, oldPos, newPos)
		}
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE mod_book_chapters SET position = $2 WHERE id = $1`, chapterID, newPos); err != nil {
			return nil, err
		}
	}

	var ch BookChapter
	err = tx.QueryRow(ctx, `
		UPDATE mod_book_chapters SET
			title = COALESCE($2, title), content = COALESCE($3, content),
			subchapter = COALESCE($4, subchapter), hidden = COALESCE($5, hidden)
		WHERE id = $1
		RETURNING id, book_id, title, content, position, subchapter, hidden, created_at
	`, chapterID, u.Title, u.Content, u.Subchapter, u.Hidden).Scan(&ch.ID, &ch.BookID, &ch.Title, &ch.Content, &ch.Position, &ch.Subchapter, &ch.Hidden, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}

	// The first chapter has nothing to nest under.
	if _, err := tx.Exec(ctx, `UPDATE mod_book_chapters SET subchapter = false WHERE book_id = $1 AND position = 0`, bookID); err != nil {
		return nil, err
	}
	if ch.Position == 0 {
		ch.Subchapter = false
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ch, nil
}

func (s *Service) DeleteBookChapter(ctx context.Context, moduleID, chapterID string) error {
	bookID, err := s.bookIDFor(ctx, moduleID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var pos int
	err = tx.QueryRow(ctx, `DELETE FROM mod_book_chapters WHERE id = $1 AND book_id = $2 RETURNING position`, chapterID, bookID).Scan(&pos)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE mod_book_chapters SET position = position - 1 WHERE book_id = $1 AND position > $2`, bookID, pos); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE mod_book_chapters SET subchapter = false WHERE book_id = $1 AND position = 0`, bookID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
