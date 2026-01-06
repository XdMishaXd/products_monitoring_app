package getProducts

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	resp "main_service/internal/lib/api/response"
	sl "main_service/internal/lib/logger"
	authMiddlware "main_service/internal/middleware/auth"
	"main_service/internal/models"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
)

const (
	defaultLimit  = 20
	maxLimit      = 100
	defaultOffset = 0
)

type Response struct {
	resp.Response
	Products   []models.Product `json:"products"`
	Pagination Pagination       `json:"pagination"`
}

type Pagination struct {
	Limit      int64 `json:"limit"`
	Offset     int64 `json:"offset"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
	HasMore    bool  `json:"has_more"`
}

type ProductsGetter interface {
	Products(ctx context.Context, userID, limit, offset int64) ([]models.Product, int64, error)
}

func New(
	log *slog.Logger,
	productsGetter ProductsGetter,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.products.get.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		limit := parseLimit(r)
		offset := parseOffset(r)

		userID, ok := r.Context().Value(authMiddlware.UserIDKey).(int64)
		if !ok {
			log.Error("User ID not found in context")

			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("Unauthorized"))

			return
		}

		if userID <= 0 {
			log.Error("Invalid user ID", slog.Int64("user_id", userID))

			render.Status(r, http.StatusUnauthorized)
			render.JSON(w, r, resp.Error("Unauthorized"))

			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		products, total, err := productsGetter.Products(ctx, userID, limit, offset)
		if err != nil {
			log.Error("Failed to get products",
				sl.Err(err),
				slog.Int64("user_id", userID),
				slog.Int64("limit", limit),
				slog.Int64("offset", offset),
			)

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("Internal error"))

			return
		}

		if products == nil {
			products = []models.Product{}
		}

		log.Info("Products retrieved successfully",
			slog.Int64("user_id", userID),
			slog.Int("count", len(products)),
			slog.Int64("total", total),
		)

		w.Header().Set("Cache-Control", "private, max-age=60")

		log.Info("Products got successfully", slog.Int64("userID", userID))

		ResponseOK(w, r, products, limit, offset, total)
	}
}

func ResponseOK(w http.ResponseWriter, r *http.Request, products []models.Product, limit, offset, total int64) {
	render.JSON(w, r, Response{
		Response: resp.OK(),
		Products: products,
		Pagination: Pagination{
			Limit:      limit,
			Offset:     offset,
			Total:      total,
			TotalPages: (total + limit - 1) / limit,
			HasMore:    offset+int64(len(products)) < total,
		},
	})
}

func parseLimit(r *http.Request) int64 {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit
	}

	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limit <= 0 {
		return defaultLimit
	}

	if limit > maxLimit {
		return maxLimit
	}

	return limit
}

func parseOffset(r *http.Request) int64 {
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		return defaultOffset
	}

	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil || offset < 0 {
		return defaultOffset
	}

	return offset
}
