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

type EbayParser struct {
	client *http.Client
}

func NewEbayParser() *EbayParser {
	return &EbayParser{
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

// * Parse парсит страницу товара на eBay и возвращает цену и наличие
func (p *EbayParser) Parse(ctx context.Context, product models.Product) (*models.ParsedProduct, error) {
	const op = "parsers.EbayParser.Parse"

	if !strings.Contains(product.URL, "ebay.com") {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, parsers.ErrInvalidURL)
	}

	// * Формирование запроса к ebay.com
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, product.URL, nil)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to create request: %w", op, err)
	}

	// * Заголовки для имитации браузера
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
		ID:       product.ID,
		Currency: currency,
		Price:    price,
	}

	// Парсим наличие
	info.InStock = p.parseAvailability(doc)

	return info, nil
}

// * ParseWithRetry парсит с повторными попытками
func (p *EbayParser) ParseWithRetry(
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

// * parsePrice извлекает цену из документа
func (p *EbayParser) parsePriceAndCurrency(doc *goquery.Document, htmlBody string) (float32, string, error) {
	var priceText string
	var currencyCode string

	for _, selector := range parsers.PriceSelectorsEbay {
		if priceText != "" {
			break
		}

		doc.Find(selector).Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			text := strings.TrimSpace(s.Text())

			if parsers.ExactPricePatternEbay.MatchString(text) {
				priceText = text
				currencyCode = parsers.ExtractCurrencyFromText(text)
			}
		})
	}

	if priceText == "" {
		doc.Find("div.x-price-primary, [data-testid='x-price-primary']").Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			text := strings.TrimSpace(s.Text())
			if len(text) < 20 && parsers.ExactPricePatternEbay.MatchString(text) {
				priceText = text
				currencyCode = parsers.ExtractCurrencyFromText(text)
			}
		})
	}

	if priceText == "" {
		doc.Find("script[type='application/ld+json']").Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			jsonText := s.Text()

			if currencyCode == "" {
				if match := parsers.CurrencyCodePattern.FindStringSubmatch(jsonText); len(match) > 1 {
					currencyCode = match[1]
				}
			}

			for _, pattern := range parsers.JsonPatternsEbay {
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

	if currencyCode == "" {
		if content, exists := doc.Find("meta[property='product:price:currency']").First().Attr("content"); exists {
			currencyCode = strings.ToUpper(strings.TrimSpace(content))
		}
	}

	if priceText == "" {
		doc.Find("[data-testid*='price']").Each(func(i int, s *goquery.Selection) {
			if priceText != "" {
				return
			}

			text := strings.TrimSpace(s.Text())
			if match := parsers.DataTestIDPriceEbay.FindString(text); match != "" {
				testPrice := strings.ReplaceAll(strings.TrimPrefix(match, "$"), ",", "")

				if price, err := strconv.ParseFloat(testPrice, 64); err == nil {
					if price >= 1.0 && price <= 100000 {
						priceText = match
						if currencyCode == "" {
							currencyCode = parsers.ExtractCurrencyFromText(text)
						}
					}
				}
			}
		})
	}

	if priceText == "" {
		for _, selector := range parsers.MetaSelectorsEbay {
			if content, exists := doc.Find(selector).First().Attr("content"); exists {
				if strings.Contains(content, ".") {
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

	if priceText == "" {
		for _, selector := range parsers.OldSelectorsEbay {
			if priceText != "" {
				break
			}

			s := doc.Find(selector).First()
			if s.Length() > 0 {
				text := strings.TrimSpace(s.Text())

				if parsers.ExactPricePatternEbay.MatchString(text) {
					priceText = text
					if currencyCode == "" {
						currencyCode = parsers.ExtractCurrencyFromText(text)
					}
				}
			}
		}
	}

	if currencyCode == "" {
		currencyCode = parsers.DetectCurrencyByDomain(htmlBody)
	}

	if currencyCode == "" {
		currencyCode = "USD"
	}

	price, err := cleanAndParsePrice(priceText)
	if err != nil {
		return 0, "", err
	}

	return price, currencyCode, nil
}

// * parseAvailability проверяет наличие товара
func (p *EbayParser) parseAvailability(doc *goquery.Document) bool {
	for _, selector := range parsers.BuyButtonSelectorsEbay {
		elements := doc.Find(selector)

		if elements.Length() > 0 {
			return true
		}
	}

	for _, selector := range parsers.QuantityInputSelectorsEbay {
		if doc.Find(selector).Length() > 0 {
			return true
		}
	}

	for _, selector := range parsers.AvailabilityContainersEbay {
		element := doc.Find(selector).First()
		if element.Length() > 0 {
			text := strings.ToLower(strings.TrimSpace(element.Text()))

			// Проверяем позитивные индикаторы
			if strings.Contains(text, "in stock") ||
				strings.Contains(text, "available") ||
				parsers.AvailabilityPatternEbay.MatchString(text) ||
				parsers.MoreThanAvailabilityPatternEbay.MatchString(text) {
				return true
			}

			// Проверяем негативные индикаторы ТОЛЬКО в этом контейнере
			if strings.Contains(text, "out of stock") ||
				strings.Contains(text, "sold out") ||
				strings.Contains(text, "no longer available") {
				return false
			}
		}
	}

	for _, selector := range parsers.PriceContainersEbay {
		element := doc.Find(selector).First()
		if element.Length() > 0 {
			text := strings.ToLower(strings.TrimSpace(element.Text()))

			if strings.Contains(text, "out of stock") {
				return false
			}
		}
	}

	title := strings.ToLower(doc.Find("title").Text())
	if strings.Contains(title, "no longer available") ||
		strings.Contains(title, "listing has ended") {
		return false
	}

	pageText := strings.ToLower(doc.Text())

	for _, indicator := range parsers.CriticalIndicatorsEbay {
		if strings.Contains(pageText, indicator) {
			return false
		}
	}

	hasPositive := false
	for _, indicator := range parsers.PositiveIndicatorsEbay {
		if strings.Contains(pageText, indicator) {
			hasPositive = true
			break
		}
	}

	if hasPositive {
		return true
	}

	return true
}

// * cleanAndParsePrice очищает строку цены и конвертирует в float32
func cleanAndParsePrice(priceText string) (float32, error) {
	priceText = strings.TrimSpace(priceText)

	for _, r := range parsers.ReplacementsEbay {
		priceText = strings.ReplaceAll(priceText, r, "")
	}

	priceText = strings.ReplaceAll(priceText, " ", "")
	priceText = strings.ReplaceAll(priceText, "\n", "")
	priceText = strings.ReplaceAll(priceText, "\t", "")

	priceText = strings.ReplaceAll(priceText, ",", "")

	priceText = strings.TrimSpace(priceText)

	matches := parsers.NumberPatternEbay.FindString(priceText)

	if matches == "" {
		return 0, fmt.Errorf("no valid number found in price text: '%s'", priceText)
	}

	priceFloat, err := strconv.ParseFloat(matches, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price '%s': %w", matches, err)
	}

	if priceFloat < 0.01 || priceFloat > 10000000 {
		return 0, fmt.Errorf("price out of reasonable range: %.2f", priceFloat)
	}

	return float32(priceFloat), nil
}
