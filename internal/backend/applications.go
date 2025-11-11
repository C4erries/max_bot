package backend

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
	"sync"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/schemes"
	"github.com/rs/zerolog"
)

// Role определяет тип пользователя в контексте заявок.
type Role string

// ApplicationType описывает доступные типы заявок.
type ApplicationType string

const (
	RoleStudent Role = "student"
	RoleTeacher Role = "teacher"

	ApplicationTypeStudyCertificate ApplicationType = "study_certificate"
	ApplicationTypeAcademicLeave    ApplicationType = "academic_leave"
	ApplicationTypeStudyTransfer    ApplicationType = "study_transfer"
	ApplicationTypeWorkCertificate  ApplicationType = "work_certificate"
)

// Applications определяет контракт общения с backend для заявок.
type Applications interface {
	ResolveRole(ctx context.Context, userID int64) (Role, error)
	SubmitApplication(ctx context.Context, userID int64, role Role, docType ApplicationType, payload map[string]string) error
}

// MockApplications расширяет контракт хранением загруженных файлов (используется в e2e).
type MockApplications interface {
	Applications
	StoredFiles(userID int64) map[string][]schemes.FileAttachment
}

// NewApplications возвращает HTTP-бэкенд или встроенный стаб, если baseURL пуст.
func NewApplications(baseURL string, log zerolog.Logger) (Applications, error) {
	if strings.TrimSpace(baseURL) == "" {
		return newStubApplications(log), nil
	}
	return newHTTPApplications(baseURL, log)
}

type httpApplications struct {
	baseURL string
	client  *http.Client
	log     zerolog.Logger
}

func newHTTPApplications(baseURL string, log zerolog.Logger) (*httpApplications, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return nil, errors.New("application backend: base url is empty")
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
		return nil, errors.New("application backend: resolved base url is empty")
	}

	return &httpApplications{
		baseURL: cleaned,
		client:  &http.Client{Timeout: 10 * time.Second},
		log:     log.With().Str("component", "application-backend").Logger(),
	}, nil
}

func (b *httpApplications) ResolveRole(ctx context.Context, userID int64) (Role, error) {
	path := fmt.Sprintf("/api/users/%d/role", userID)
	var resp roleResponse
	if err := b.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return "", err
	}
	role := Role(strings.ToLower(strings.TrimSpace(resp.Role)))
	if !isSupportedRole(role) {
		return "", fmt.Errorf("backend returned unsupported role %q", resp.Role)
	}
	return role, nil
}

func (b *httpApplications) SubmitApplication(ctx context.Context, userID int64, role Role, docType ApplicationType, payload map[string]string) error {
	reqBody := submitRequest{
		UserID:  userID,
		Role:    role,
		Type:    docType,
		Payload: payload,
	}
	return b.doRequest(ctx, http.MethodPost, "/api/applications/submissions", reqBody, nil)
}

func (b *httpApplications) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
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
	Role    Role              `json:"role"`
	Type    ApplicationType   `json:"type"`
	Payload map[string]string `json:"payload"`
}

func isSupportedRole(role Role) bool {
	switch role {
	case RoleStudent, RoleTeacher:
		return true
	default:
		return false
	}
}

type stubApplications struct {
	log         zerolog.Logger
	mu          sync.Mutex
	submissions map[int64]map[string][]schemes.FileAttachment
}

func newStubApplications(log zerolog.Logger) *stubApplications {
	return &stubApplications{
		log:         log.With().Str("component", "documents-mock").Logger(),
		submissions: make(map[int64]map[string][]schemes.FileAttachment),
	}
}

func (s *stubApplications) ResolveRole(_ context.Context, userID int64) (Role, error) {
	if userID%2 == 0 {
		return RoleStudent, nil
	}
	return RoleTeacher, nil
}

func (s *stubApplications) SubmitApplication(_ context.Context, userID int64, role Role, docType ApplicationType, payload map[string]string) error {
	s.log.Info().
		Int64("user_id", userID).
		Str("role", string(role)).
		Str("doc_type", string(docType)).
		Msg("mock document submission")

	attachments := s.extractAttachments(payload)
	if len(attachments) == 0 {
		return nil
	}

	s.mu.Lock()
	s.submissions[userID] = attachments
	s.mu.Unlock()
	return nil
}

func (s *stubApplications) StoredFiles(userID int64) map[string][]schemes.FileAttachment {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, ok := s.submissions[userID]
	if !ok {
		return nil
	}
	copy := make(map[string][]schemes.FileAttachment, len(files))
	for field, list := range files {
		copy[field] = append([]schemes.FileAttachment(nil), list...)
	}
	return copy
}

func (s *stubApplications) extractAttachments(payload map[string]string) map[string][]schemes.FileAttachment {
	result := make(map[string][]schemes.FileAttachment)
	for field, rawValue := range payload {
		data := strings.TrimSpace(rawValue)
		if data == "" || !strings.HasPrefix(data, "[") {
			continue
		}
		files := decodeFileAttachmentPayload(field, data, s.log)
		if len(files) == 0 {
			continue
		}
		result[field] = files
	}
	return result
}

func decodeFileAttachmentPayload(field, data string, log zerolog.Logger) []schemes.FileAttachment {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		log.Debug().Err(err).Str("field", field).Msg("mock backend failed to parse attachment array")
		return nil
	}

	var attachments []schemes.FileAttachment
	for _, item := range items {
		var file schemes.FileAttachment
		if err := json.Unmarshal(item, &file); err != nil {
			log.Debug().Err(err).Msg("mock backend failed to decode attachment")
			continue
		}
		if file.Filename == "" {
			log.Debug().Msg("mock backend skipping attachment without filename")
			continue
		}
		log.Info().
			Str("field", field).
			Str("filename", file.Filename).
			Int64("size", file.Size).
			Str("token", file.Payload.Token).
			Msg("mock backend received file payload")
		attachments = append(attachments, file)
	}
	return attachments
}

var _ Applications = (*httpApplications)(nil)
var _ Applications = (*stubApplications)(nil)
var _ MockApplications = (*stubApplications)(nil)
