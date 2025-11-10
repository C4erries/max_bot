package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type applicationRole string

type applicationType string

const (
	roleStudent applicationRole = "student"
	roleTeacher applicationRole = "teacher"

	applicationTypeStudyCertificate applicationType = "study_certificate"
	applicationTypeAcademicLeave    applicationType = "academic_leave"
	applicationTypeStudyTransfer    applicationType = "study_transfer"
	applicationTypeWorkCertificate  applicationType = "work_certificate"
)

type fieldKind string

const (
	fieldKindText fieldKind = "text"
	fieldKindFile fieldKind = "file"
)

// applicationField описывает отдельный шаг формы.
type applicationField struct {
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Placeholder string    `json:"placeholder,omitempty"`
	Required    bool      `json:"required"`
	Kind        fieldKind `json:"kind"`
}

type applicationForm struct {
	Title  string
	Fields []applicationField
}

// applicationBackend определяет контракт общения с внешним сервисом.
type applicationBackend interface {
	ResolveRole(ctx context.Context, userID int64) (applicationRole, error)
	SubmitApplication(ctx context.Context, userID int64, role applicationRole, docType applicationType, payload map[string]string) error
}

// applicationCoordinator отвечает за выдачу форм и общение с backend.
type applicationCoordinator struct {
	backend applicationBackend
	forms   map[applicationType]applicationForm
}

func newApplicationCoordinator(baseURL string, log zerolog.Logger) (*applicationCoordinator, error) {
	backend, err := newHTTPApplicationBackend(baseURL, log)
	if err != nil {
		return nil, err
	}
	return &applicationCoordinator{
		backend: backend,
		forms:   defaultApplicationForms(),
	}, nil
}

func (c *applicationCoordinator) ResolveRole(ctx context.Context, userID int64) (applicationRole, error) {
	if c.backend == nil {
		return "", errors.New("application coordinator backend is not configured")
	}
	return c.backend.ResolveRole(ctx, userID)
}

// PrepareSession подбирает форму по типу документа и возвращает состояние первого шага.
func (c *applicationCoordinator) PrepareSession(_ int64, role applicationRole, docType applicationType) (applicationSessionData, error) {
	form, ok := c.forms[docType]
	if !ok {
		return applicationSessionData{}, fmt.Errorf("application form %q is not configured", docType)
	}

	fields := make([]applicationField, len(form.Fields))
	copy(fields, form.Fields)

	return applicationSessionData{
		Role:      role,
		Type:      docType,
		FormTitle: form.Title,
		Fields:    fields,
		Values:    make(map[string]string, len(fields)),
	}, nil
}

func (c *applicationCoordinator) Submit(ctx context.Context, userID int64, data applicationSessionData) error {
	if c.backend == nil {
		return errors.New("application coordinator backend is not configured")
	}
	if data.Values == nil {
		data.Values = make(map[string]string)
	}
	return c.backend.SubmitApplication(ctx, userID, data.Role, data.Type, data.Values)
}

type applicationSessionData struct {
	Role      applicationRole    `json:"role"`
	Type      applicationType    `json:"type"`
	FormTitle string             `json:"form_title"`
	Fields    []applicationField `json:"fields"`
	Index     int                `json:"index"`
	Values    map[string]string  `json:"values"`
}

func (d *applicationSessionData) marshal() ([]byte, error) {
	d.ensureValues()
	return json.Marshal(d)
}

func applicationSessionFromPayload(payload []byte) (applicationSessionData, error) {
	if len(payload) == 0 {
		return applicationSessionData{}, fmt.Errorf("application payload is empty")
	}

	var data applicationSessionData
	if err := json.Unmarshal(payload, &data); err != nil {
		return applicationSessionData{}, err
	}
	if data.Values == nil {
		data.Values = make(map[string]string)
	}
	return data, nil
}

func (d *applicationSessionData) ensureValues() {
	if d.Values == nil {
		d.Values = make(map[string]string)
	}
}

func (d applicationSessionData) currentField() (applicationField, bool) {
	if d.Index < 0 || d.Index >= len(d.Fields) {
		return applicationField{}, false
	}
	return d.Fields[d.Index], true
}

func (d applicationSessionData) StepsCount() int {
	return len(d.Fields)
}

func (d applicationSessionData) StartPrompt() string {
	return d.buildPrompt(true)
}

func (d applicationSessionData) NextPrompt() string {
	return d.buildPrompt(false)
}

func (d applicationSessionData) buildPrompt(includeIntro bool) string {
	field, ok := d.currentField()
	if !ok {
		return ""
	}

	var b strings.Builder
	if includeIntro {
		fmt.Fprintf(&b, "Открыта форма «%s».\n", d.FormTitle)
		fmt.Fprintf(&b, "Всего %d %s.\n\n", len(d.Fields), pluralizeQuestions(len(d.Fields)))
	}
	b.WriteString(renderFieldPrompt(field, d.Index, len(d.Fields)))
	return b.String()
}

func (d applicationSessionData) ReminderForRequiredField() string {
	field, ok := d.currentField()
	if !ok {
		return "Это поле обязательно для заполнения."
	}
	return fmt.Sprintf("Поле «%s» обязательно для заполнения.\n\n%s", field.Label, renderFieldPrompt(field, d.Index, len(d.Fields)))
}

func (d *applicationSessionData) RecordAnswer(value string) {
	field, ok := d.currentField()
	if !ok {
		return
	}
	d.ensureValues()
	d.Values[field.Name] = value
	d.Index++
}

// RecordFileAnswer сохраняет сериализованную информацию о вложениях.
func (d *applicationSessionData) RecordFileAnswer(payload string) {
	field, ok := d.currentField()
	if !ok {
		return
	}
	d.ensureValues()
	d.Values[field.Name] = payload
	d.Index++
}

func (d applicationSessionData) IsCompleted() bool {
	return d.Index >= len(d.Fields)
}

func renderFieldPrompt(field applicationField, index, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Шаг %d/%d.\n%s", index+1, total, field.Label)
	if field.Required {
		b.WriteString(" (обязательно)")
	}
	if field.Kind == fieldKindFile {
		b.WriteString("\nОтправьте файл или несколько файлов следующим сообщением.")
	}
	if field.Placeholder != "" {
		fmt.Fprintf(&b, "\nПодсказка: %s", field.Placeholder)
	}
	return b.String()
}

func pluralizeQuestions(n int) string {
	if n%10 == 1 && n%100 != 11 {
		return "вопрос"
	}
	if (n%10 >= 2 && n%10 <= 4) && (n%100 < 12 || n%100 > 14) {
		return "вопроса"
	}
	return "вопросов"
}

// defaultApplicationForms клонирует базовый набор форм, чтобы можно было
// безопасно модифицировать их в рантайме по конкретным сценариям.
func defaultApplicationForms() map[applicationType]applicationForm {
	forms := make(map[applicationType]applicationForm, len(fixedApplicationForms))
	for key, form := range fixedApplicationForms {
		forms[key] = form.clone()
	}
	return forms
}

// fixedApplicationForms — статическая конфигурация шагов для MVP.
var fixedApplicationForms = map[applicationType]applicationForm{
	applicationTypeStudyCertificate: {
		Title: "Справка об обучении",
	},
	applicationTypeAcademicLeave: {
		Title: "Академический отпуск",
		Fields: []applicationField{
			{
				Name:     "supporting_files",
				Label:    "Прикрепите документы, подтверждающие причину оформления",
				Kind:     fieldKindFile,
				Required: true,
			},
			{
				Name:        "reason_text",
				Label:       "Опишите причину академического отпуска",
				Placeholder: "Например, длительное лечение",
				Kind:        fieldKindText,
				Required:    true,
			},
		},
	},
	applicationTypeStudyTransfer: {
		Title: "Перевод на другое направление",
		Fields: []applicationField{
			{
				Name:     "gradebook_copy",
				Label:    "Загрузите копию зачетной книжки",
				Kind:     fieldKindFile,
				Required: true,
			},
			{
				Name:        "target_program",
				Label:       "Укажите факультет и направление, куда хотите перевестись",
				Placeholder: "Например, ФКН — Прикладная информатика",
				Kind:        fieldKindText,
				Required:    true,
			},
		},
	},
	applicationTypeWorkCertificate: {
		Title: "Справка с места работы",
	},
}

func (f applicationForm) clone() applicationForm {
	clone := applicationForm{Title: f.Title}
	if len(f.Fields) > 0 {
		clone.Fields = make([]applicationField, len(f.Fields))
		copy(clone.Fields, f.Fields)
	}
	return clone
}

func formatSuccessMessage(title string) string {
	return fmt.Sprintf("Заявка «%s» отправлена. Как только появится ответ — мы сообщим вам в этом чате.", title)
}

type httpApplicationBackend struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPApplicationBackend(baseURL string, log zerolog.Logger) (*httpApplicationBackend, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return nil, fmt.Errorf("application backend: base url is empty")
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("application backend: parse base url: %w", err)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	cleaned := strings.TrimRight(parsed.String(), "/")
	if cleaned == "" {
		return nil, fmt.Errorf("application backend: resolved base url is empty")
	}

	return &httpApplicationBackend{
		baseURL: cleaned,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "application-backend").Logger(),
	}, nil
}

func (b *httpApplicationBackend) ResolveRole(ctx context.Context, userID int64) (applicationRole, error) {
	path := fmt.Sprintf("/api/users/%d/role", userID)
	var resp roleResponse
	if err := b.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	role := applicationRole(strings.ToLower(strings.TrimSpace(resp.Role)))
	if !isSupportedRole(role) {
		return "", fmt.Errorf("backend returned unsupported role %q", resp.Role)
	}
	return role, nil
}

func (b *httpApplicationBackend) SubmitApplication(ctx context.Context, userID int64, role applicationRole, docType applicationType, payload map[string]string) error {
	reqBody := submitRequest{
		UserID:  userID,
		Role:    role,
		Type:    docType,
		Payload: payload,
	}
	return b.doRequest(ctx, http.MethodPost, "/api/applications/submissions", reqBody, nil)
}

func (b *httpApplicationBackend) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	fullURL := fmt.Sprintf("%s%s", b.baseURL, path)

	var reqBody io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("backend %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode backend response: %w", err)
	}
	return nil
}

type roleResponse struct {
	Role string `json:"role"`
}

type submitRequest struct {
	UserID  int64             `json:"user_id"`
	Role    applicationRole   `json:"role"`
	Type    applicationType   `json:"type"`
	Payload map[string]string `json:"payload"`
}

func isSupportedRole(role applicationRole) bool {
	switch role {
	case roleStudent, roleTeacher:
		return true
	default:
		return false
	}
}
