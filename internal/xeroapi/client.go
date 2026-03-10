package xeroapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cesar/xero-cli/internal/auth"
	appconfig "github.com/cesar/xero-cli/internal/config"
	clierrors "github.com/cesar/xero-cli/internal/errors"
)

const defaultBaseURL = "https://api.xero.com"

var xeroDatePattern = regexp.MustCompile(`^/Date\((\d+)([+-]\d{4})?\)/$`)

type Invoice struct {
	InvoiceID     string  `json:"invoiceId"`
	InvoiceNumber string  `json:"invoiceNumber"`
	ContactName   string  `json:"contactName,omitempty"`
	Status        string  `json:"status,omitempty"`
	Total         float64 `json:"total,omitempty"`
	AmountDue     float64 `json:"amountDue,omitempty"`
	Currency      string  `json:"currency,omitempty"`
	DueDate       string  `json:"dueDate,omitempty"`
	UpdatedAt     string  `json:"updatedAt,omitempty"`
}

type ListInvoicesRequest struct {
	TenantID string
	Status   string
	Contact  string
	Since    string
	Page     int
	Limit    int
}

type InvoiceLister interface {
	ListInvoices(context.Context, auth.TokenSet, ListInvoicesRequest) ([]Invoice, error)
}

type ClientOptions struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	settings   appconfig.Settings
	baseURL    string
	httpClient *http.Client
}

type invoicesResponse struct {
	Invoices []invoicePayload `json:"Invoices"`
}

type invoicePayload struct {
	InvoiceID      string         `json:"InvoiceID"`
	InvoiceNumber  string         `json:"InvoiceNumber"`
	Contact        contactPayload `json:"Contact"`
	Status         string         `json:"Status"`
	Total          float64        `json:"Total"`
	AmountDue      float64        `json:"AmountDue"`
	CurrencyCode   string         `json:"CurrencyCode"`
	DueDate        string         `json:"DueDate"`
	UpdatedDateUTC string         `json:"UpdatedDateUTC"`
}

type contactPayload struct {
	Name string `json:"Name"`
}

type apiErrorPayload struct {
	Type    string `json:"Type"`
	Title   string `json:"Title"`
	Detail  string `json:"Detail"`
	Message string `json:"Message"`
	Error   string `json:"error"`
}

func NewClient(settings appconfig.Settings, options ClientOptions) *Client {
	baseURL := strings.TrimRight(options.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{settings: settings, baseURL: baseURL, httpClient: httpClient}
}

func (c *Client) ListInvoices(ctx context.Context, token auth.TokenSet, request ListInvoicesRequest) ([]Invoice, error) {
	endpoint, err := url.Parse(c.baseURL + "/api.xro/2.0/Invoices")
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoices URL", err)
	}

	query := endpoint.Query()
	if request.Status != "" {
		query.Set("Statuses", request.Status)
	}
	if request.Contact != "" {
		query.Set("searchTerm", request.Contact)
	}
	if request.Page > 0 {
		query.Set("page", strconv.Itoa(request.Page))
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "build Xero invoices request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Xero-tenant-id", request.TenantID)
	if request.Since != "" {
		since, err := time.Parse("2006-01-02", request.Since)
		if err != nil {
			return nil, clierrors.Wrap(clierrors.KindValidation, "parse --since date", err)
		}
		req.Header.Set("If-Modified-Since", since.UTC().Format(time.RFC1123))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.KindNetwork, "send Xero invoices request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var payload apiErrorPayload
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		message := firstNonEmpty(payload.Detail, payload.Message, payload.Error, payload.Title, resp.Status)
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return nil, clierrors.New(clierrors.KindRateLimit, message)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, clierrors.New(clierrors.KindAuthRequired, message)
		default:
			return nil, clierrors.New(clierrors.KindXeroAPI, message)
		}
	}

	var payload invoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, clierrors.Wrap(clierrors.KindXeroRequest, "decode Xero invoices response", err)
	}

	invoices := make([]Invoice, 0, len(payload.Invoices))
	for _, item := range payload.Invoices {
		invoices = append(invoices, Invoice{
			InvoiceID:     item.InvoiceID,
			InvoiceNumber: item.InvoiceNumber,
			ContactName:   item.Contact.Name,
			Status:        item.Status,
			Total:         item.Total,
			AmountDue:     item.AmountDue,
			Currency:      item.CurrencyCode,
			DueDate:       normalizeDate(item.DueDate),
			UpdatedAt:     normalizeTimestamp(item.UpdatedDateUTC),
		})
	}

	if request.Limit > 0 && len(invoices) > request.Limit {
		invoices = invoices[:request.Limit]
	}
	return invoices, nil
}

func normalizeDate(raw string) string {
	if raw == "" {
		return ""
	}
	if value, ok := parseXeroWrappedDate(raw); ok {
		return value.UTC().Format("2006-01-02")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format("2006-01-02")
		}
	}
	return raw
}

func normalizeTimestamp(raw string) string {
	if raw == "" {
		return ""
	}
	if value, ok := parseXeroWrappedDate(raw); ok {
		return value.UTC().Format(time.RFC3339)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return raw
}

func parseXeroWrappedDate(raw string) (time.Time, bool) {
	matches := xeroDatePattern.FindStringSubmatch(raw)
	if len(matches) == 0 {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

func NewRequestSummary(count int) string {
	if count == 1 {
		return "1 invoice"
	}
	return fmt.Sprintf("%d invoices", count)
}

func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 90*time.Second)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
