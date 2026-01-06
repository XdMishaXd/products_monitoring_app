package getByID

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

type Response struct {
	resp.Response
	Product models.Product `json:"product"`
}

type ProductGetter interface {
	ProductByID(ctx context.Context, productID int64) (models.Product, error)
}

func New(
	log *slog.Logger,
	prodOp ProductGetter,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.products.delete.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		productID := parseProductID(r)
		if productID == -1 {
			log.Error("Invalid id")

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("Invalid id"))

			return
		}

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

		product, err := prodOp.ProductByID(ctx, productID)
		if err != nil {
			log.Error("Failed to get product",
				sl.Err(err),
				slog.Int64("user_id", userID),
				slog.Int64("productID", productID),
			)

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("Internal error"))

			return
		}

		w.Header().Set("Cache-Control", "private, max-age=60")

		log.Info("Products got successfully", slog.Int64("userID", userID))

		ResponseOK(w, r, product)
	}
}

func ResponseOK(w http.ResponseWriter, r *http.Request, product models.Product) {
	render.JSON(w, r, Response{
		Response: resp.OK(),
		Product:  product,
	})
}

func parseProductID(r *http.Request) int64 {
	productIDStr := r.URL.Query().Get("id")
	if productIDStr == "" {
		return -1
	}

	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil || productID < 0 {
		return -1
	}

	return productID
}
