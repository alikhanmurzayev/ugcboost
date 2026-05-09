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
	"sort"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// rateLimitRPS is per-blueprint requirement: «Не более 4 запросов в
	// секунду». golang.org/x/time/rate.Limiter с Burst=1 даёт sliding-window
	// без накопления budget'а.
	rateLimitRPS = 4
	// defaultTimeout — TrustMe staging иногда отдаёт 10+ сек на large PDF;
	// 60 сек — потолок.
	defaultTimeout = 60 * time.Second
)

// RealClient — HTTP-реализация Client против test.trustme.kz (или production
// host'а, когда выдадут). Обёрнут rate-limiter'ом (4 RPS per blueprint).
type RealClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	limiter    *rate.Limiter
}

// NewRealClient создаёт клиента под живой TrustMe. baseURL — без trailing
// slash; token — статичный. nil-httpClient → http.DefaultClient.
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
// FIO, IIN_BIN, PhoneNumber). wire отличается от Requisite только json-тегами,
// поэтому Go-конверсия структуры в структуру допустима.
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
	URL        string `json:"url"`
	DocumentID string `json:"document_id"`
	FileName   string `json:"fileName"`
}

// SendToSign отправляет multipart/form-data POST на
// /SendToSignBase64FileExt/pdf. Возвращает выданный TrustMe document_id +
// short_url + file_name из data.url/document_id/fileName.
func (c *RealClient) SendToSign(ctx context.Context, in SendToSignInput) (*SendToSignResult, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}

	details := sendToSignDetails{
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

	// auto_sign — компания-инициатор (UGCBoost, зашита в токен) подписывается
	// автоматически после загрузки документа. Платный функционал TrustMe
	// (blueprint § Отправка документа с автоподписанием), требует активации
	// в их кабинете; без активации возвращается 1219. Сейчас выключено
	// (auto_sign=0), пока TrustMe не подтвердят, что у нас активировано.
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
		return nil, fmt.Errorf("trustme: send-to-sign http %d: %s", resp.StatusCode, truncate(respBody))
	}

	var wrapper apiResponse[sendToSignData]
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal send-to-sign: %w", err)
	}
	if !strings.EqualFold(wrapper.Status, "Ok") {
		return nil, fmt.Errorf("trustme: send-to-sign status=%s: %s", wrapper.Status, FormatErrorText(wrapper.ErrorText))
	}
	var data sendToSignData
	if err := json.Unmarshal(wrapper.Data, &data); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal send-to-sign data: %w", err)
	}
	if data.DocumentID == "" {
		// Diagnostic: если TrustMe вернул success-wrapper без document_id, важно
		// видеть, что именно лежит в data — возможно поле называется иначе или
		// прислано пустым. Логируем имена ключей + technical поля, без значений
		// (security.md: PII-фрейминг — не пробрасываем сырой body, который
		// потенциально содержит echo Requisites).
		dataKeys := dataObjectKeys(wrapper.Data)
		return nil, fmt.Errorf("trustme: send-to-sign returned empty document_id (status=%q errorText=%q url=%q fileName=%q dataKeys=%v)",
			wrapper.Status, FormatErrorText(wrapper.ErrorText), data.URL, data.FileName, dataKeys)
	}
	return &SendToSignResult{
		DocumentID: data.DocumentID,
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

// SearchContractByAdditionalInfo — POST /search/Contracts с фильтром
// fieldName="additionalInfo". Возвращает первый совпавший элемент;
// ErrTrustMeNotFound — если массив пустой. Используется outbox-worker'ом в
// Phase 0 recovery: если TrustMe знает наш additionalInfo, finalize'им без
// re-send'а; если нет — перепосылаем.
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
		return nil, fmt.Errorf("trustme: search http %d: %s", resp.StatusCode, truncate(respBody))
	}
	var wrapper apiResponse[[]searchContractItem]
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("trustme: unmarshal search: %w", err)
	}
	if !strings.EqualFold(wrapper.Status, "Ok") {
		return nil, fmt.Errorf("trustme: search status=%s: %s", wrapper.Status, FormatErrorText(wrapper.ErrorText))
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
		c.baseURL+"/doc/DownloadContractFile/"+documentID, nil)
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
		return nil, fmt.Errorf("trustme: download http %d: %s", resp.StatusCode, truncate(body))
	}
	return io.ReadAll(resp.Body)
}

func truncate(b []byte) string {
	const max = 256
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}

// dataObjectKeys возвращает отсортированные ключи top-level JSON object'а из
// raw. Если raw — не object (например []), возвращает пустой слайс. Используется
// в diagnostic-error при empty document_id, чтобы увидеть, под каким именем
// TrustMe прислал ID, не светя при этом значения (PII-safety).
func dataObjectKeys(raw json.RawMessage) []string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
