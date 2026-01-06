package parsers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"parsing_service/internal/models"

	"github.com/PuerkitoBio/goquery"
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrPriceNotFound   = errors.New("price not found")
	ErrInvalidURL      = errors.New("invalid URL")
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
				DisableCompression:  false,
				DisableKeepAlives:   false,
				MaxIdleConnsPerHost: 10,
			},
		},
	}
}

// * Parse парсит страницу товара на eBay и возвращает цену и наличие
func (p *EbayParser) Parse(ctx context.Context, product models.Product) (*models.ParsedProduct, error) {
	const op = "parsers.EbayParser.Parse"

	if !strings.Contains(product.URL, "ebay.com") {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, ErrInvalidURL)
	}

	// * Формирование запроса к ebay.com
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, product.URL, nil)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: failed to create request: %w", op, err)
	}

	// * Заголовки для имитации браузера
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

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

	// Парсим цену
	price, err := p.parsePrice(doc)
	if err != nil {
		return &models.ParsedProduct{}, fmt.Errorf("%s: %w", op, err)
	}

	info := &models.ParsedProduct{
		ID:    product.ID,
		Price: price,
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

		if errors.Is(err, ErrInvalidURL) || errors.Is(err, ErrProductNotFound) {
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
func (p *EbayParser) parsePrice(doc *goquery.Document) (float32, error) {
	// * Селекторы для цены
	selectors := []string{
		".x-price-primary span.ux-textspans",
		".x-price-primary .ux-textspans--BOLD",
		"div[data-testid='x-price-primary'] span",
		".mainPrice .ux-textspans",
		"span[itemprop='price']",
		".x-bin-price__content .ux-textspans",
	}

	var priceText string
	for _, selector := range selectors {
		priceText = doc.Find(selector).First().Text()
		if priceText != "" {
			break
		}
	}

	// * Поиск цены по regexp
	if priceText == "" {
		htmlText, _ := doc.Html()

		re := regexp.MustCompile(`"price":\s*"([\d,\.]+)"`)
		matches := re.FindStringSubmatch(htmlText)

		if len(matches) > 1 {
			priceText = matches[1]
		}
	}

	if priceText == "" {
		return 0, ErrPriceNotFound
	}

	// Очищаем текст цены от лишних символов
	priceText = strings.TrimSpace(priceText)
	priceText = strings.ReplaceAll(priceText, "US", "")
	priceText = strings.ReplaceAll(priceText, "$", "")
	priceText = strings.ReplaceAll(priceText, "€", "")
	priceText = strings.ReplaceAll(priceText, "£", "")
	priceText = strings.ReplaceAll(priceText, ",", "")
	priceText = strings.TrimSpace(priceText)

	priceFloat, err := strconv.ParseFloat(priceText, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price '%s': %w", priceText, err)
	}

	return float32(priceFloat), nil
}

// * parseAvailability проверяет наличие товара
func (p *EbayParser) parseAvailability(doc *goquery.Document) bool {
	// * Индикаторы отсутствия товара
	unavailableIndicators := []string{
		"out of stock",
		"sold out",
		"unavailable",
		"no longer available",
		"this listing has ended",
		"this item is out of stock",
	}

	pageText := strings.ToLower(doc.Text())

	for _, indicator := range unavailableIndicators {
		if strings.Contains(pageText, indicator) {
			return false
		}
	}

	// Проверка кнопок "Add to cart" и "Buy It Now"
	buyButtonSelectors := []string{
		"a[data-testid='ux-call-to-action']",
		"a.ux-call-to-action",
		".vim-btn-primary",
		"[data-testid='x-atc-cta-btn']",
	}

	for _, selector := range buyButtonSelectors {
		if doc.Find(selector).Length() > 0 {
			return true
		}
	}

	// * Проверка количества доступных товаров
	quantitySelectors := []string{
		"span[data-testid='qtySubTxt'] span.ux-textspans--BOLD",
		".qtyTxt .ux-textspans--BOLD",
		".vi-qty-pur-lnk",
	}

	for _, selector := range quantitySelectors {
		qtyText := doc.Find(selector).First().Text()

		if qtyText != "" {
			qtyText = strings.ToLower(strings.TrimSpace(qtyText))

			if strings.Contains(qtyText, "available") ||
				strings.Contains(qtyText, "left") ||
				strings.Contains(qtyText, "in stock") {

				return true
			}
		}
	}

	// * По умолчанию товар в наличии, если не найдено явных признаков отсутствия
	return true
}
