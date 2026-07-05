package delivery

import (
	"errors"
	"mime"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/services/media/internal/api/domain"
	"github.com/pizdagladki/full/services/media/internal/api/repository"
	"github.com/pizdagladki/full/services/media/internal/api/service"
)

// kingClipUploadResponse is the JSON body returned by the king-clip upload
// endpoint.
type kingClipUploadResponse struct {
	ID int64 `json:"id"`
}

// kingClipCurrentResponse is the JSON body returned by the current-king-clip
// endpoint.
type kingClipCurrentResponse struct {
	DownloadURL string `json:"download_url"`
	BlinkTsMs   int64  `json:"blink_ts_ms"`
}

type kingClipHandler struct {
	svc      service.KingClipService
	maxBytes int64
	logger   *zap.Logger
}

// NewKingClipHandler returns a KingClipHandler wired to the given service.
func NewKingClipHandler(svc service.KingClipService, maxBytes int64, logger *zap.Logger) KingClipHandler {
	return &kingClipHandler{svc: svc, maxBytes: maxBytes, logger: logger}
}

// Upload handles POST /v1/king-clips. hill_type and blink_ts_ms are read from
// the query string; the request body is the raw WebM.
func (h *kingClipHandler) Upload(c echo.Context) error {
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

	hillType := c.QueryParam("hill_type")
	if !domain.ValidHillType(hillType) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hill type"})
	}

	blinkTsMs, err := strconv.ParseInt(c.QueryParam("blink_ts_ms"), 10, 64)
	if err != nil || blinkTsMs < 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid blink_ts_ms"})
	}

	size := c.Request().ContentLength
	if size <= 0 || size > h.maxBytes {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
	}

	// Wrap body so an overflowing body can't silently exceed the limit.
	body := http.MaxBytesReader(c.Response(), c.Request().Body, h.maxBytes)

	clip, err := h.svc.Upload(c.Request().Context(), userID, hillType, blinkTsMs, rawCT, size, body)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidHillType):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hill type"})
		case errors.Is(err, domain.ErrInvalidContentType):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid content type"})
		case errors.Is(err, domain.ErrTooLarge):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
		case errors.Is(err, domain.ErrInvalidBlinkTs):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid blink_ts_ms"})
		default:
			// Check if the error is a MaxBytesError (body exceeded limit).
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "file too large"})
			}

			h.logger.Error("upload king clip", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return c.JSON(http.StatusCreated, kingClipUploadResponse{ID: clip.ID})
}

// Current handles GET /v1/king-clips/current.
func (h *kingClipHandler) Current(c echo.Context) error {
	_, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	hillType := c.QueryParam("hill_type")

	url, blinkTsMs, err := h.svc.CurrentURL(c.Request().Context(), hillType)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidHillType):
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid hill type"})
		case errors.Is(err, repository.ErrKingClipNotFound):
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		default:
			h.logger.Error("current king clip", zap.Error(err))

			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	return c.JSON(http.StatusOK, kingClipCurrentResponse{DownloadURL: url, BlinkTsMs: blinkTsMs})
}

// Delete handles DELETE /v1/king-clips/:id.
func (h *kingClipHandler) Delete(c echo.Context) error {
	userID, ok := c.Get(UserIDContextKey).(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid king clip id"})
	}

	err = h.svc.Delete(c.Request().Context(), userID, id)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}

		h.logger.Error("delete king clip", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.NoContent(http.StatusNoContent)
}

// DeleteInternal handles DELETE /internal/v1/king-clips/:id. Unlike Delete,
// it is reached only through the internalauth-gated group (no user session
// in context) and does NOT read UserIDContextKey — it expires the king clip
// unconditionally, on behalf of a trusted internal caller (the koth reset
// worker).
func (h *kingClipHandler) DeleteInternal(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid king clip id"})
	}

	err = h.svc.ExpireByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrKingClipNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
		}

		h.logger.Error("expire king clip", zap.Error(err))

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	return c.NoContent(http.StatusNoContent)
}
