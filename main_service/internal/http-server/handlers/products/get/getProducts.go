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

// New godoc
// @Summary      Получить список отслеживаемых товаров
// @Description  ## Описание
// @Description  Возвращает список всех товаров, которые отслеживает текущий пользователь, с поддержкой пагинации.
// @Description
// @Description  ### Процесс получения:
// @Description  1. Извлечение параметров пагинации (limit, offset) из query
// @Description  2. Валидация параметров (limit <= 100, offset >= 0)
// @Description  3. Проверка авторизации (JWT токен)
// @Description  4. Извлечение user_id из токена
// @Description  5. Получение списка товаров из базы данных
// @Description  6. Формирование метаданных пагинации
// @Description  7. Возврат результата с заголовком Cache-Control
// @Description
// @Description  ### Параметры пагинации:
// @Description  - **limit**: Количество товаров на странице (по умолчанию: 20, максимум: 100)
// @Description  - **offset**: Количество товаров для пропуска (по умолчанию: 0)
// @Description
// @Description  ### Формула пагинации:
// @Description  - Страница 1: `offset=0, limit=20`
// @Description  - Страница 2: `offset=20, limit=20`
// @Description  - Страница 3: `offset=40, limit=20`
// @Description
// @Description  ### Метаданные ответа:
// @Description  - **total**: Общее количество товаров пользователя
// @Description  - **total_pages**: Общее количество страниц
// @Description  - **has_more**: Есть ли еще товары (для бесконечного скролла)
// @Description
// @Description  ### Кэширование:
// @Description  - Ответ кэшируется на клиенте на 60 секунд (Cache-Control: private, max-age=60)
// @Description  - Private cache означает что только браузер пользователя кэширует ответ
// @Description
// @Description  ### Структура товара:
// @Description  Каждый товар содержит:
// @Description  - ID, URL, название
// @Description  - Маркетплейс (etsy, ebay, aliexpress)
// @Description  - Текущая цена и валюта
// @Description  - История изменений цены
// @Description  - Дата добавления и последнего обновления
// @Tags         products
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        limit   query  int  false  "Количество товаров на странице"  minimum(1)  maximum(100)  default(20)  example(20)
// @Param        offset  query  int  false  "Количество товаров для пропуска"  minimum(0)  default(0)  example(0)
// @Success      200  {object}  object{status=string,products=[]object,pagination=object{limit=int,offset=int,total=int,total_pages=int,has_more=bool}}  "Список товаров с метаданными пагинации"
// @Failure      401  {object}  object{status=string,error=string}  "Требуется авторизация: отсутствует или невалидный JWT токен"  example({"status": "error", "error": "Unauthorized"})
// @Failure      500  {object}  object{status=string,error=string}  "Внутренняя ошибка сервера"  example({"status": "error", "error": "Internal error"})
// @Router       /products [get]
// @x-order      2
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
