package parsers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"parsing_service/internal/models"
	"parsing_service/internal/parsers"

	"github.com/PuerkitoBio/goquery"
)

type EtsyParser struct {
	client *http.Client
}

func NewEtsyParser() *EtsyParser {
	return &EtsyParser{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

// * Parse парсит страницу товара на Etsy и возвращает цену и наличие
func (p *EtsyParser) Parse(ctx context.Context, product models.Product) (*models.ParsedProduct, error) {
	const op = "parsers.EtsyParser.Parse"

	if !strings.Contains(product.URL, "etsy.com") {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, parsers.ErrInvalidURL)
	}

	// Формирование запроса к etsy.com
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, product.URL, nil)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to create request: %w", op, err)
	}

	// Заголовки для имитации браузера
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := p.client.Do(req)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to fetch page: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &models.ParsedProduct{}, fmt.Errorf("%s: unexpected status code: %d", op, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to read body: %w", op, err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to parse HTML: %w", op, err)
	}

	// Парсим цену и валюту
	price, currency, err := p.parsePriceAndCurrency(doc, string(body))
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, err)
	}

	info := &models.ParsedProduct{
		ID:         product.ID,
		CurrencyID: models.CurrencyIDs[currency],
		Price:      price,
	}

	// Парсим наличие
	info.InStock = p.parseAvailability(doc)

	return info, nil
}

// * ParseWithRetry парсит с повторными попытками
func (p *EtsyParser) ParseWithRetry(
	ctx context.Context,
	product models.Product,
	maxRetries int,
) (*models.ParsedProduct, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		info, err := p.Parse(ctx, product)
		if err == nil {
			return info, nil
		}

		lastErr = err

		if errors.Is(err, parsers.ErrInvalidURL) || errors.Is(err, parsers.ErrProductNotFound) {
			return nil, err
		}

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * time.Duration(i+1)):
				// Экспоненциальная задержка
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// * parsePriceAndCurrency извлекает цену и валюту из документа
func (p *EtsyParser) parsePriceAndCurrency(doc *goquery.Document, htmlBody string) (float32, string, error) {
	var priceText string
	var currencyCode string

	// Попытка 1: Основные селекторы цены Etsy
	for _, selector := range parsers.PriceSelectorsEtsy {
		if priceText != "" {
			break
		}

		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			text := strings.TrimSpace(s.Text())

			if parsers.PricePatternEtsy.MatchString(text) {
				priceText = text
				currencyCode = parsers.ExtractCurrencyFromText(text)
			}
		})
	}

	// Попытка 2: Data-атрибуты
	if priceText == "" {
		doc.Find("[data-buy-box-region='price']").Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			if price, exists := s.Attr("data-price"); exists && price != "" {
				priceText = price
			} else {
				text := strings.TrimSpace(s.Text())
				if parsers.PricePatternEtsy.MatchString(text) {
					priceText = text
				}
			}

			if currency, exists := s.Attr("data-currency"); exists && currency != "" {
				currencyCode = currency
			} else if currencyCode == "" {
				currencyCode = parsers.ExtractCurrencyFromText(s.Text())
			}
		})
	}

	// Попытка 3: JSON-LD структурированные данные
	if priceText == "" {
		doc.Find("script[type='application/ld+json']").Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			jsonText := s.Text()

			// Извлекаем валюту
			if currencyCode == "" {
				if match := parsers.CurrencyCodePattern.FindStringSubmatch(jsonText); len(match) > 1 {
					currencyCode = match[1]
				}
			}

			// Извлекаем цену из различных JSON полей
			for _, pattern := range parsers.JsonPatternsEtsy {
				if matches := pattern.FindStringSubmatch(jsonText); len(matches) > 1 {
					testPrice := strings.ReplaceAll(matches[1], ",", "")
					if price, err := strconv.ParseFloat(testPrice, 64); err == nil {
						if price >= 0.01 && price <= 1000000 {
							priceText = matches[1]
							return
						}
					}
				}
			}
		})
	}

	// Попытка 4: Meta теги
	if priceText == "" {
		for _, selector := range parsers.MetaSelectorsEtsy {
			if content, exists := doc.Find(selector).First().Attr("content"); exists {
				if strings.Contains(content, ".") || strings.Contains(content, ",") {
					testPrice := strings.ReplaceAll(content, ",", "")

					if price, err := strconv.ParseFloat(testPrice, 64); err == nil {
						if price >= 0.01 && price <= 1000000 {
							priceText = content
							break
						}
					}
				}
			}
		}
	}

	// Попытка 5: Извлечение валюты из meta тегов
	if currencyCode == "" {
		if content, exists := doc.Find("meta[property='product:price:currency']").First().Attr("content"); exists {
			currencyCode = strings.ToUpper(strings.TrimSpace(content))
		}
	}

	if currencyCode == "" {
		if content, exists := doc.Find("meta[property='og:price:currency']").First().Attr("content"); exists {
			currencyCode = strings.ToUpper(strings.TrimSpace(content))
		}
	}

	// Попытка 6: Старые селекторы (для обратной совместимости)
	if priceText == "" {
		for _, selector := range parsers.OldSelectorsEtsy {
			if priceText != "" {
				break
			}

			s := doc.Find(selector).First()
			if s.Length() > 0 {
				text := strings.TrimSpace(s.Text())

				if parsers.PricePatternEtsy.MatchString(text) {
					priceText = text
					if currencyCode == "" {
						currencyCode = parsers.ExtractCurrencyFromText(text)
					}
				}
			}
		}
	}

	// Определение валюты по домену, если не найдена
	if currencyCode == "" {
		currencyCode = parsers.DetectCurrencyByDomain(htmlBody)
	}

	// Валюта по умолчанию
	if currencyCode == "" {
		currencyCode = "USD"
	}

	price, err := cleanAndParsePriceEtsy(priceText)
	if err != nil {
		return 0, "", err
	}

	return price, currencyCode, nil
}

// * parseAvailability проверяет наличие товара
func (p *EtsyParser) parseAvailability(doc *goquery.Document) bool {
	// Проверка 1: Кнопка "Add to cart"
	for _, selector := range parsers.AddToCartSelectorsEtsy {
		elements := doc.Find(selector)

		if elements.Length() > 0 {
			// Проверяем, что кнопка не отключена
			if disabled, exists := elements.First().Attr("disabled"); !exists || disabled == "" {
				return true
			}
		}
	}

	// Проверка 2: Поле выбора количества
	for _, selector := range parsers.QuantitySelectorsEtsy {
		if doc.Find(selector).Length() > 0 {
			return true
		}
	}

	// Проверка 3: Контейнеры с информацией о наличии
	for _, selector := range parsers.AvailabilitySelectorsEtsy {
		element := doc.Find(selector).First()
		if element.Length() > 0 {
			text := strings.ToLower(strings.TrimSpace(element.Text()))

			// Позитивные индикаторы
			if strings.Contains(text, "in stock") ||
				strings.Contains(text, "available") ||
				strings.Contains(text, "ready to ship") ||
				parsers.QuantityAvailablePatternEtsy.MatchString(text) {
				return true
			}

			// Негативные индикаторы
			if strings.Contains(text, "out of stock") ||
				strings.Contains(text, "sold out") ||
				strings.Contains(text, "unavailable") ||
				strings.Contains(text, "no longer available") {
				return false
			}
		}
	}

	// Проверка 4: Проверка data-атрибутов
	if availability, exists := doc.Find("[data-listing-availability]").First().Attr("data-listing-availability"); exists {
		availability = strings.ToLower(strings.TrimSpace(availability))
		if availability == "available" || availability == "in_stock" {
			return true
		}
		if availability == "unavailable" || availability == "out_of_stock" {
			return false
		}
	}

	// Проверка 5: Заголовок страницы
	title := strings.ToLower(doc.Find("title").Text())
	if strings.Contains(title, "no longer available") ||
		strings.Contains(title, "sold out") ||
		strings.Contains(title, "unavailable") {
		return false
	}

	// Проверка 6: Критические индикаторы на странице
	pageText := strings.ToLower(doc.Text())

	for _, indicator := range parsers.CriticalIndicatorsEtsy {
		if strings.Contains(pageText, indicator) {
			return false
		}
	}

	// Проверка 7: Позитивные индикаторы
	hasPositive := false
	for _, indicator := range parsers.PositiveIndicatorsEtsy {
		if strings.Contains(pageText, indicator) {
			hasPositive = true
			break
		}
	}

	if hasPositive {
		return true
	}

	// По умолчанию считаем товар доступным, если нашли цену
	return true
}

// * cleanAndParsePriceEtsy очищает строку цены и конвертирует в float32
func cleanAndParsePriceEtsy(priceText string) (float32, error) {
	priceText = strings.TrimSpace(priceText)

	// Удаляем текстовые метки
	for _, r := range parsers.ReplacementsEtsy {
		priceText = strings.ReplaceAll(priceText, r, "")
	}

	// Удаляем пробелы и специальные символы
	priceText = strings.ReplaceAll(priceText, " ", "")
	priceText = strings.ReplaceAll(priceText, "\n", "")
	priceText = strings.ReplaceAll(priceText, "\t", "")
	priceText = strings.ReplaceAll(priceText, "+", "")

	// Удаляем запятые (используемые как разделители тысяч)
	priceText = strings.ReplaceAll(priceText, ",", "")

	priceText = strings.TrimSpace(priceText)

	// Извлекаем числовое значение
	matches := parsers.NumberPatternEtsy.FindString(priceText)

	if matches == "" {
		return 0, fmt.Errorf("no valid number found in price text: '%s'", priceText)
	}

	priceFloat, err := strconv.ParseFloat(matches, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price '%s': %w", matches, err)
	}

	// Проверка разумности цены
	if priceFloat < 0.01 || priceFloat > 10000000 {
		return 0, fmt.Errorf("price out of reasonable range: %.2f", priceFloat)
	}

	return float32(priceFloat), nil
}
