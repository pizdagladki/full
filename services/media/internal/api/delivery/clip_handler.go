package delivery

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	"github.com/pizdagladki/full/services/media/internal/api/service"
)

// clipResponse is the JSON representation of a clip in the list endpoint.
type clipResponse struct {
	ID        int64     `json:"id"`
	Mode      string    `json:"mode"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

// uploadResponse is the JSON body returned by the upload endpoint.
type uploadResponse struct {
	ID int64 `json:"id"`
}

// downloadResponse is the JSON body returned by the download endpoint.
type downloadResponse struct {
	DownloadURL string `json:"download_url"`
}

type clipHandler struct {
	svc      service.ClipService
	maxBytes int64
	logger   *zap.Logger
}

// NewClipHandler returns a ClipHandler wired to the given service.
func NewClipHandler(svc service.ClipService, maxBytes int64, logger *zap.Logger) ClipHandler {
	return &clipHandler{svc: svc, maxBytes: maxBytes, logger: logger}
}

// Upload handles POST /v1/clips.
func (h *clipHandler) Upload(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	// Parse the media type, ignoring params like "; codecs=...".
	rawCT := c.Request().Header.Get(echo.HeaderContentType)
	mediaType, _, err := mime.ParseMediaType(rawCT)

	if err != nil || mediaType != domain.ContentTypeWebM {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid content type"})
	}

	size := c.Request().ContentLength
	if size <= 0 || size > h.maxBytes {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
	}

	// Wrap body so an overflowing body can't silently exceed the limit.
	body := http.MaxBytesReader(c.Response(), c.Request().Body, h.maxBytes)

	clip, err := h.svc.Upload(c.Request().Context(), userID, rawCT, size, body)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidContentType):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid content type"})
		case errors.Is(err, domain.ErrTooLarge):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
		default:
			// Check if the error is a MaxBytesError (body exceeded limit).
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
			}

			h.logger.Error("upload clip", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return c.JSON(http.StatusCreated, uploadResponse{ID: clip.ID})
}

// List handles GET /v1/clips.
func (h *clipHandler) List(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	clips, err := h.svc.List(c.Request().Context(), userID)
	if err != nil {
		h.logger.Error("list clips", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Always return an array, never null.
	resp := make([]clipResponse, 0, len(clips))

	for _, clip := range clips {
		resp = append(resp, clipResponse{
			ID:        clip.ID,
			Mode:      clip.Mode,
			Result:    clip.Result,
			CreatedAt: clip.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// Download handles GET /v1/clips/:id/download.
func (h *clipHandler) Download(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	idStr := c.Param("id")

	clipID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid clip id"})
	}

	url, err := h.svc.DownloadURL(c.Request().Context(), userID, clipID)
	if err != nil {
		if errors.Is(err, repository.ErrClipNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}

		h.logger.Error("download clip", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.JSON(http.StatusOK, downloadResponse{DownloadURL: url})
}

// Convert handles POST /v1/clips/:id/convert.
func (h *clipHandler) Convert(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok || userID == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	clipID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || clipID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid clip id"})
	}

	status, err := h.svc.RequestConvert(c.Request().Context(), userID, clipID)
	if err != nil {
		if errors.Is(err, repository.ErrClipNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}

		h.logger.Warn("request convert", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}

	code := http.StatusAccepted
	if status == domain.ConversionStatusDone {
		code = http.StatusOK
	}

	return c.JSON(code, map[string]string{"status": status})
}

// GetMP4 handles GET /v1/clips/:id/mp4.
func (h *clipHandler) GetMP4(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok || userID == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	clipID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || clipID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid clip id"})
	}

	url, err := h.svc.GetMP4URL(c.Request().Context(), userID, clipID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrClipNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		case errors.Is(err, domain.ErrConversionNotDone):
			return c.JSON(http.StatusConflict, map[string]string{"error": "conversion not complete"})
		case errors.Is(err, domain.ErrConversionFailed):
			return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "conversion failed"})
		default:
			h.logger.Warn("get mp4 url", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"mp4_url": url})
}
