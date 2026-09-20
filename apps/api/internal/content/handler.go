package content

import (
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"polaris-api/internal/auth"
	"polaris-api/internal/coursemodule"
)

// maxUploadSize caps multipart bodies for resource/folder file uploads.
// Plenty for small course documents; revisit if a later module needs
// large media.
const maxUploadSize = 32 << 20 // 32MB

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CourseIDForSection/CourseIDForModule let the router resolve the RBAC
// context for nested routes (section/module id in the path, but the
// capability check needs the owning course's context).
func (h *Handler) CourseIDForSection(r *http.Request, sectionID string) (string, error) {
	return h.service.CourseIDForSection(r.Context(), sectionID)
}

func (h *Handler) CourseIDForModule(r *http.Request, moduleID string) (string, error) {
	return h.service.CourseIDForModule(r.Context(), moduleID)
}

func (h *Handler) CourseContent(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	sections, err := h.service.CourseContent(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load course content")
		return
	}
	writeJSON(w, http.StatusOK, sections)
}

type labelRequest struct {
	Content string `json:"content"`
}

func (h *Handler) CreateLabel(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req labelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	mod, label, err := h.service.CreateLabel(r.Context(), r.PathValue("sectionId"), userID, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create label")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "label": label})
}

type pageRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (h *Handler) CreatePage(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req pageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	mod, page, err := h.service.CreatePage(r.Context(), r.PathValue("sectionId"), userID, req.Title, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create page")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "page": page})
}

type urlRequest struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func (h *Handler) CreateURL(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req urlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.URL = strings.TrimSpace(req.URL)
	if req.Title == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "title and url are required")
		return
	}

	mod, u, err := h.service.CreateURL(r.Context(), r.PathValue("sectionId"), userID, req.Title, req.URL, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create url")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "url": u})
}

func (h *Handler) CreateResource(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	mod, res, err := h.service.CreateResource(r.Context(), r.PathValue("sectionId"), userID, title, header.Filename, contentTypeOf(header), header.Size, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create resource")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "resource": res})
}

type folderRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req folderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	mod, folder, err := h.service.CreateFolder(r.Context(), r.PathValue("sectionId"), userID, req.Title, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create folder")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "folder": folder})
}

func (h *Handler) AddFolderFile(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserIDFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	ff, err := h.service.AddFolderFile(r.Context(), r.PathValue("moduleId"), r.FormValue("path"), header.Filename, contentTypeOf(header), header.Size, file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add file to folder")
		return
	}
	writeJSON(w, http.StatusCreated, ff)
}

type bookRequest struct {
	Title string `json:"title"`
	Intro string `json:"intro"`
}

func (h *Handler) CreateBook(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req bookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	mod, book, err := h.service.CreateBook(r.Context(), r.PathValue("sectionId"), userID, req.Title, req.Intro)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create book")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"module": mod, "book": book})
}

type chapterRequest struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Subchapter bool   `json:"subchapter"`
}

func (h *Handler) AddBookChapter(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserIDFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req chapterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	chapter, err := h.service.AddBookChapter(r.Context(), r.PathValue("moduleId"), req.Title, req.Content, req.Subchapter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add chapter")
		return
	}
	writeJSON(w, http.StatusCreated, chapter)
}

// UpdateModule edits the generic module settings shared by every module type
// (visibility, intro, group mode, availability, position).
func (h *Handler) UpdateModule(w http.ResponseWriter, r *http.Request) {
	var u coursemodule.Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.service.modules.Update(r.Context(), r.PathValue("moduleId"), u)
	if h.writeModuleError(w, err, "could not update module") {
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// UpdateModuleContent edits the fields owned by the module's own type.
func (h *Handler) UpdateModuleContent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.writeModuleError(w, h.service.UpdateContent(r.Context(), r.PathValue("moduleId"), body), "could not update module content") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteModule(w http.ResponseWriter, r *http.Request) {
	if h.writeModuleError(w, h.service.modules.Delete(r.Context(), r.PathValue("moduleId")), "could not delete module") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateFolderFile(w http.ResponseWriter, r *http.Request) {
	var u FolderFileUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.writeModuleError(w, h.service.UpdateFolderFile(r.Context(), r.PathValue("moduleId"), r.PathValue("fileId"), u), "could not update file") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteFolderFile(w http.ResponseWriter, r *http.Request) {
	if h.writeModuleError(w, h.service.DeleteFolderFile(r.Context(), r.PathValue("moduleId"), r.PathValue("fileId")), "could not delete file") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateBookChapter(w http.ResponseWriter, r *http.Request) {
	var u ChapterUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ch, err := h.service.UpdateBookChapter(r.Context(), r.PathValue("moduleId"), r.PathValue("chapterId"), u)
	if h.writeModuleError(w, err, "could not update chapter") {
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

func (h *Handler) DeleteBookChapter(w http.ResponseWriter, r *http.Request) {
	if h.writeModuleError(w, h.service.DeleteBookChapter(r.Context(), r.PathValue("moduleId"), r.PathValue("chapterId")), "could not delete chapter") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeModuleError maps module errors to HTTP statuses and reports whether
// it wrote a response.
func (h *Handler) writeModuleError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, coursemodule.ErrModuleNotFound), errors.Is(err, ErrNotFound), errors.Is(err, coursemodule.ErrSectionNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidContent), errors.Is(err, coursemodule.ErrInvalidGroupMode),
		errors.Is(err, coursemodule.ErrInvalidGrouping), errors.Is(err, coursemodule.ErrInvalidWindow),
		errors.Is(err, coursemodule.ErrCrossCourseMove):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
	return true
}

func contentTypeOf(header *multipart.FileHeader) string {
	if ct := header.Header.Get("Content-Type"); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
