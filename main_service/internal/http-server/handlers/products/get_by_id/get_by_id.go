package getByID

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	resp "main_service/internal/lib/api/response"
	sl "main_service/internal/lib/logger"
	authMiddlware "main_service/internal/middleware/auth"
	"main_service/internal/models"
	"main_service/internal/storage"

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

// New godoc
// @Summary      Получить товар по ID
// @Description  ## Описание
// @Description  Возвращает детальную информацию о конкретном товаре по его ID.
// @Description
// @Description  ### Процесс получения:
// @Description  1. Извлечение product_id из query параметра `id`
// @Description  2. Валидация ID (должен быть положительным числом)
// @Description  3. Проверка авторизации (JWT токен)
// @Description  4. Получение товара из базы данных
// @Description  5. Проверка статуса парсинга товара
// @Description  6. Возврат детальной информации
// @Description
// @Description  ### Информация о товаре:
// @Description  - Базовая информация: ID, URL, название, маркетплейс
// @Description  - Текущая цена и валюта
// @Description  - История изменений цены (массив объектов с ценой и датой)
// @Description  - Статистика: минимальная, максимальная и средняя цена
// @Description  - Даты: добавления товара и последнего обновления
// @Description  - Статус парсинга (успешно, в процессе, ошибка)
// @Description
// @Description  ### Статусы парсинга:
// @Description  - **409 Conflict**: Товар добавлен, но еще не распарсен (подождите 1-2 минуты)
// @Description  - **500 Parsing error**: Ошибка при парсинге (недоступен сайт или изменилась структура)
// @Description  - **200 OK**: Товар успешно распарсен, данные актуальны
// @Description
// @Description  ### Кэширование:
// @Description  - Ответ кэшируется на клиенте на 60 секунд
// @Description  - Private cache (только для конкретного пользователя)
// @Description  - При изменении цены кэш автоматически инвалидируется
// @Description
// @Description  ### История цен:
// @Description  Используется для построения графиков изменения цены во времени.
// @Description  Содержит все зафиксированные изменения с момента добавления товара.
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  query  int  true  "ID товара"  minimum(1)  example(42)
// @Success      200  {object}  object{status=string,product=object}  "Детальная информация о товаре"
// @Failure      400  {object}  object{status=string,error=string}  "Некорректный ID: отсутствует, не число или отрицательное"  example({"status": "error", "error": "Invalid id"})
// @Failure      401  {object}  object{status=string,error=string}  "Требуется авторизация"  example({"status": "error", "error": "Unauthorized"})
// @Failure      404  {object}  object{status=string,error=string}  "Товар не найден"  example({"status": "error", "error": "Product not found"})
// @Failure      409  {object}  object{status=string,error=string}  "Товар еще не распарсен, подождите"  example({"status": "error", "error": "Product not yet received"})
// @Failure      500  {object}  object{status=string,error=string}  "Ошибка парсинга или внутренняя ошибка"  example({"status": "error", "error": "Parsing error"})
// @Router       /product [get]
// @x-order      4
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
			if errors.Is(err, storage.ErrParsedProductNotYetRecieved) {
				log.Info("Product not yet received")

				render.Status(r, http.StatusConflict)
				render.JSON(w, r, resp.Error("Product not yet received"))

				return
			}

			if errors.Is(err, storage.ErrParsingFailed) {
				log.Info("Failed to parse product")

				render.Status(r, http.StatusInternalServerError)
				render.JSON(w, r, resp.Error("Parsing error"))

				return
			}

			if errors.Is(err, storage.ErrProductNotFound) {
				log.Info("Product not found")

				render.Status(r, http.StatusNotFound)
				render.JSON(w, r, resp.Error("Product not found"))

				return
			}

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
