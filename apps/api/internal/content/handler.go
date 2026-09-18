package content

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"

	"polaris-api/internal/auth"
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
	courseID := r.PathValue("id")
	sections, err := h.service.CourseContent(r.Context(), courseID)
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

	ff, err := h.service.AddFolderFile(r.Context(), r.PathValue("moduleId"), header.Filename, contentTypeOf(header), header.Size, file)
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
	Title   string `json:"title"`
	Content string `json:"content"`
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

	chapter, err := h.service.AddBookChapter(r.Context(), r.PathValue("moduleId"), req.Title, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add chapter")
		return
	}
	writeJSON(w, http.StatusCreated, chapter)
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
