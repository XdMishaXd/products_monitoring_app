package addProduct

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	resp "main_service/internal/lib/api/response"
	sl "main_service/internal/lib/logger"
	authMiddlware "main_service/internal/middleware/auth"
	"main_service/internal/middleware/products"
	"main_service/internal/models"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	validator "github.com/go-playground/validator/v10"
)

type Request struct {
	URL   string `json:"url" validate:"required,url"`
	Title string `json:"title" validate:"required"`
}

type Response struct {
	resp.Response
	ProductID int64 `json:"product_id"`
}

func New(
	log *slog.Logger,
	prodOp *products.ProductOperator,
	validate *validator.Validate,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.products.add.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // * 1 МБ лимит запроса
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			log.Error("Failed to decode request body", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("Failed to decode request"))

			return
		}

		log.Info("Request body decoded")

		if err := validate.Struct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)

			log.Error("Invalid request", sl.Err(err))

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.ValidationError(validateErr))

			return
		}

		marketplace := parseMarketplace(req.URL)
		if marketplace == "" {
			log.Error("Marketplace undefined")

			render.Status(r, http.StatusBadRequest)
			render.JSON(w, r, resp.Error("Marketplace undefined"))

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

		productID, err := prodOp.SaveProduct(ctx, req.URL, req.Title, userID, marketplace)
		if err != nil {
			log.Error("Failed to save product", sl.Err(err))

			render.Status(r, http.StatusInternalServerError)
			render.JSON(w, r, resp.Error("Internal error"))

			return
		}

		log.Info("Product saved successfully",
			slog.Int64("product_id", productID),
			slog.Int64("user_id", userID),
		)

		render.Status(r, http.StatusCreated)
		ResponseOK(w, r, productID)
	}
}

func parseMarketplace(urlStr string) models.Marketplace {
	u, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	host := strings.ToLower(u.Hostname())

	switch {
	case strings.Contains(host, "etsy.com"):
		return models.Etsy
	case strings.Contains(host, "ebay.com"):
		return models.Ebay
	case strings.Contains(host, "aliexpress.com") || strings.Contains(host, "aliexpress.ru"):
		return models.Aliexpress
	default:
		return ""
	}
}

func ResponseOK(w http.ResponseWriter, r *http.Request, id int64) {
	render.JSON(w, r, Response{
		Response:  resp.OK(),
		ProductID: id,
	})
}
