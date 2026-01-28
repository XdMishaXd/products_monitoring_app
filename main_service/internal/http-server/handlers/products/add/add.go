package addProduct

import (
	"context"
	"errors"
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
	"main_service/internal/storage"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	validator "github.com/go-playground/validator/v10"
)

type Request struct {
	URL   string `json:"url" validate:"required,url,product_url"`
	Title string `json:"title" validate:"required"`
}

type Response struct {
	resp.Response
	ProductID int64 `json:"product_id"`
}

// New godoc
// @Summary      Добавить товар для отслеживания
// @Description  ## Описание
// @Description  Добавляет товар из маркетплейса в список отслеживаемых для мониторинга цен.
// @Description
// @Description  ### Процесс добавления:
// @Description  1. Валидация URL товара и названия
// @Description  2. Определение маркетплейса по URL (Etsy, eBay, AliExpress)
// @Description  3. Извлечение user_id из JWT токена (Authorization header)
// @Description  4. Проверка что товар еще не отслеживается пользователем
// @Description  5. Сохранение товара в базу данных
// @Description  6. Запуск фонового мониторинга цены
// @Description
// @Description  ### Поддерживаемые маркетплейсы:
// @Description  - **Etsy**: `https://www.etsy.com/listing/...`
// @Description  - **eBay**: `https://www.ebay.com/itm/...`
// @Description  - **AliExpress**: `https://aliexpress.com/item/...` или `https://aliexpress.ru/item/...`
// @Description
// @Description  ### URL Requirements:
// @Description  - Валидный URL формат (протокол + домен)
// @Description  - Ссылка должна вести на страницу товара
// @Description  - Маркетплейс должен поддерживаться системой
// @Description
// @Description  ### Мониторинг цен:
// @Description  После добавления товара система:
// @Description  - Автоматически парсит начальную цену
// @Description  - Проверяет цену каждые N часов (настраивается)
// @Description  - Отправляет уведомления при изменении цены
// @Description  - Строит график истории цен
// @Description
// @Description  ### Лимиты:
// @Description  - Максимум 100 товаров на пользователя (можно настроить)
// @Description  - Размер запроса: 1 МБ
// @Description  - Timeout: 3 секунды
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        product  body  object{url=string,title=string}  true  "URL и название товара"  example({"url": "https://www.etsy.com/listing/123456/cool-product", "title": "Cool Product"})
// @Success      201  {object}  object{status=string,product_id=int}  "Товар добавлен в отслеживание"  example({"status": "ok", "product_id": 42})
// @Failure      409  {object}  object{status=string,error=string}  "Ошибка валидации: некорректный URL, отсутствует название или товар уже отслеживается"  example({"status": "error", "error": "Product already tracking"})
// @Failure      401  {object}  object{status=string,error=string}  "Требуется авторизация: отсутствует или невалидный JWT токен"  example({"status": "error", "error": "Unauthorized"})
// @Failure      500  {object}  object{status=string,error=string}  "Внутренняя ошибка сервера"  example({"status": "error", "error": "Internal error"})
// @Router       /product/add [post]
// @x-order      1
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
			if errors.Is(err, storage.ErrProductAlreadyExists) {
				log.Info("Product already tracking")

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("Product already tracking"))

				return
			}

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
