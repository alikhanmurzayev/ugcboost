package trustme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// rateLimitRPS — blueprint cap: «не более 4 запросов в секунду».
	rateLimitRPS   = 4
	defaultTimeout = 60 * time.Second
)

// RealClient — HTTP-реализация Client поверх test.trustme.kz (или прода).
type RealClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// NewRealClient собирает клиента. baseURL — без trailing slash. nil http.Client
// → дефолт с 60s timeout.
func NewRealClient(baseURL, token string, httpClient *http.Client) *RealClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &RealClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
		limiter:    rate.NewLimiter(rate.Limit(rateLimitRPS), 1),
	}
}

type apiResponse[T any] struct {
	Status    string          `json:"status"`
	ErrorText string          `json:"errorText"`
	Data      json.RawMessage `json:"data"`
}

func (c *RealClient) wait(ctx context.Context) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("trustme: rate-limit wait: %w", err)
	}
	return nil
}

func (c *RealClient) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trustme: http request: %w", err)
	}
	return resp, nil
}

type sendToSignDetails struct {
	NumberDial     string      `json:"NumberDial,omitempty"`
	KzBmg          bool        `json:"KzBmg"`
	FaceId         bool        `json:"FaceId"`
	AdditionalInfo string      `json:"AdditionalInfo,omitempty"`
	Requisites     []Requisite `json:"Requisites"`
}

// MarshalJSON для Requisite — TrustMe ждёт PascalCase ключи (CompanyName,
// FIO, IIN_BIN, PhoneNumber).
func (r Requisite) MarshalJSON() ([]byte, error) {
	type wire struct {
		CompanyName string `json:"CompanyName,omitempty"`
		FIO         string `json:"FIO"`
		IINBIN      string `json:"IIN_BIN"`
		PhoneNumber string `json:"PhoneNumber"`
	}
	return json.Marshal(wire(r))
}

type sendToSignData struct {
	URL string `json:"url"`
	// Реальный TrustMe возвращает `id`, а не `document_id` из blueprint.
	// Совпадает с search/Contracts → одинаковый идентификатор.
	ID       string `json:"id"`
	FileName string `json:"fileName"`
}

// SendToSign — POST /SendToSignBase64FileExt/pdf, multipart/form-data с
// FileBase64 + details JSON + contract_name.
func (c *RealClient) SendToSign(ctx context.Context, in SendToSignInput) (*SendToSignResult, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	details := sendToSignDetails{
		NumberDial:     in.NumberDial,
		AdditionalInfo: in.AdditionalInfo,
		Requisites:     in.Requisites,
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("trustme: marshal details: %w", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("FileBase64", in.PDFBase64); err != nil {
		return nil, fmt.Errorf("trustme: write FileBase64: %w", err)
	}
	if err := mw.WriteField("details", string(detailsJSON)); err != nil {
		return nil, fmt.Errorf("trustme: write details: %w", err)
	}
	if err := mw.WriteField("contract_name", in.ContractName); err != nil {
		return nil, fmt.Errorf("trustme: write contract_name: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("trustme: close multipart: %w", err)
	}

	// auto_sign=0: контрагент (креатор) подписывает первым, наша подпись
	// автоматически добавляется после. auto_sign=1 нужен только если бы мы
	// слали уже-подписанный документ, у нас не такой flow.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/SendToSignBase64FileExt/pdf?auto_sign=0", &body)
	if err != nil {
		return nil, fmt.Errorf("trustme: build send-to-sign request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trustme: read send-to-sign body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trustme: send-to-sign http %d: %s", resp.StatusCode, summarizeNon200(respBody))
	}

	var wrapper apiResponse[sendToSignData]
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal send-to-sign: %w", err)
	}
	if !strings.EqualFold(wrapper.Status, "Ok") {
		return nil, &Error{
			Code:    wrapper.ErrorText,
			Message: fmt.Sprintf("trustme: send-to-sign status=%s: %s", wrapper.Status, FormatErrorText(wrapper.ErrorText)),
		}
	}
	var data sendToSignData
	if err := json.Unmarshal(wrapper.Data, &data); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal send-to-sign data: %w", err)
	}
	if data.ID == "" {
		return nil, fmt.Errorf("trustme: send-to-sign returned empty id (status=%q errorText=%q)",
			wrapper.Status, FormatErrorText(wrapper.ErrorText))
	}
	return &SendToSignResult{
		DocumentID: data.ID,
		ShortURL:   data.URL,
		FileName:   data.FileName,
	}, nil
}

type searchContractRequest struct {
	SearchData []searchField `json:"searchData"`
	OrderField string        `json:"orderField"`
	OrderAsc   bool          `json:"orderAsc"`
	Page       int           `json:"page"`
}

type searchField struct {
	FieldName  string `json:"fieldName"`
	FieldValue string `json:"fieldValue"`
}

type searchContractItem struct {
	ID             string `json:"id"`
	ShortURL       string `json:"shortUrl"`
	ContractStatus int    `json:"contractStatus"`
	AdditionalInfo string `json:"additionalInfo"`
}

// SearchContractByAdditionalInfo — POST /search/Contracts. Phase 0
// recovery: known → finalize, ErrTrustMeNotFound → перепосылка.
func (c *RealClient) SearchContractByAdditionalInfo(ctx context.Context, additionalInfo string) (*SearchContractResult, error) {
	if additionalInfo == "" {
		return nil, errors.New("trustme: empty additionalInfo")
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	body, err := json.Marshal(searchContractRequest{
		SearchData: []searchField{{FieldName: "additionalInfo", FieldValue: additionalInfo}},
		OrderField: "CreatedAt",
		OrderAsc:   false,
		Page:       1,
	})
	if err != nil {
		return nil, fmt.Errorf("trustme: marshal search: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/search/Contracts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("trustme: build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("trustme: read search body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trustme: search http %d: %s", resp.StatusCode, summarizeNon200(respBody))
	}
	var wrapper apiResponse[[]searchContractItem]
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal search: %w", err)
	}
	if !strings.EqualFold(wrapper.Status, "Ok") {
		return nil, &Error{
			Code:    wrapper.ErrorText,
			Message: fmt.Sprintf("trustme: search status=%s: %s", wrapper.Status, FormatErrorText(wrapper.ErrorText)),
		}
	}
	var items []searchContractItem
	if len(wrapper.Data) > 0 {
		if err := json.Unmarshal(wrapper.Data, &items); err != nil {
			return nil, fmt.Errorf("trustme: unmarshal search data: %w", err)
		}
	}
	for _, it := range items {
		if it.AdditionalInfo == additionalInfo {
			return &SearchContractResult{
				DocumentID:     it.ID,
				ShortURL:       it.ShortURL,
				ContractStatus: it.ContractStatus,
			}, nil
		}
	}
	return nil, ErrTrustMeNotFound
}

// DownloadContractFile возвращает byte slice с подписанным PDF.
func (c *RealClient) DownloadContractFile(ctx context.Context, documentID string) ([]byte, error) {
	if documentID == "" {
		return nil, errors.New("trustme: empty document_id")
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/doc/DownloadContractFile/"+url.PathEscape(documentID), nil)
	if err != nil {
		return nil, fmt.Errorf("trustme: build download request: %w", err)
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trustme: download http %d: %s", resp.StatusCode, summarizeNon200(body))
	}
	return io.ReadAll(resp.Body)
}

// summarizeNon200 — диагностика error.Message при HTTP non-200. Парсит body
// как apiResponse → `status="..." errorText="..."`; иначе — первые 64 байта
// raw. Не пробрасываем весь body: он попадает в contracts.last_error_message
// и может содержать echo Requisites (PII).
func summarizeNon200(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var probe apiResponse[json.RawMessage]
	if err := json.Unmarshal(b, &probe); err == nil && (probe.Status != "" || probe.ErrorText != "") {
		return fmt.Sprintf("status=%q errorText=%q", probe.Status, FormatErrorText(probe.ErrorText))
	}
	const max = 64
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
