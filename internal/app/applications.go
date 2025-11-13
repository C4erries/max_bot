package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/c4erries/max_bot/internal/backend"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type applicationRole = backend.Role

type applicationType = backend.ApplicationType

const (
	roleStudent applicationRole = backend.RoleStudent
	roleTeacher applicationRole = backend.RoleTeacher

	applicationTypeStudyCertificate applicationType = backend.ApplicationTypeStudyCertificate
	applicationTypeAcademicLeave    applicationType = backend.ApplicationTypeAcademicLeave
	applicationTypeStudyTransfer    applicationType = backend.ApplicationTypeStudyTransfer
	applicationTypeWorkCertificate  applicationType = backend.ApplicationTypeWorkCertificate
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

// applicationCoordinator отвечает за выдачу форм и общение с backend.
type applicationCoordinator struct {
	backend     backend.Applications
	forms       map[applicationType]applicationForm
	mockBackend backend.MockApplications
}

func newApplicationCoordinator(appBackend backend.Applications) (*applicationCoordinator, error) {
	if appBackend == nil {
		return nil, errors.New("application backend is nil")
	}

	var mock backend.MockApplications
	if mv, ok := appBackend.(backend.MockApplications); ok {
		mock = mv
	}
	return &applicationCoordinator{
		backend:     appBackend,
		forms:       defaultApplicationForms(),
		mockBackend: mock,
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

func (c *applicationCoordinator) MockSubmittedFiles(userID int64) map[string][]schemes.FileAttachment {
	if c == nil || c.mockBackend == nil {
		return nil
	}
	return c.mockBackend.StoredFiles(userID)
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
	fmt.Fprintf(&b, "Шаг %d/%d.\n\n%s", index+1, total, field.Label)
	if field.Required {
		b.WriteString(" [обязательно]")
	}
	if field.Kind == fieldKindFile {
		b.WriteString("\n\nОтправьте файл или несколько файлов следующим сообщением.")
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
				Label:    "Чтобы мы могли начать обработку вашей заявки, пожалуйста, пришлите в этот чат фотографии или сканы документов, подтверждающих основание для ухода в академический отпуск (например, медицинская справка, повестка в армию и т.д.).",
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
	return fmt.Sprintf("✅ Заявка принята!\n\nТип документа: %s\nСтатус: ⌛️ Отправлена на обработку\n\nМы уведомим вас о результате в этом чате, как только получим ответ от отдела кадров.", title)
}
