package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockApplicationBackend struct {
	role              applicationRole
	submittedUserID   int64
	submittedRole     applicationRole
	submittedDocType  applicationType
	submittedPayload  map[string]string
	submitCallCount   int
	resolveCallCount  int
	resolveShouldFail error
	submitShouldFail  error
}

func (m *mockApplicationBackend) ResolveRole(_ context.Context, _ int64) (applicationRole, error) {
	m.resolveCallCount++
	if m.resolveShouldFail != nil {
		return "", m.resolveShouldFail
	}
	if m.role == "" {
		return roleStudent, nil
	}
	return m.role, nil
}

func (m *mockApplicationBackend) SubmitApplication(_ context.Context, userID int64, role applicationRole, docType applicationType, payload map[string]string) error {
	m.submitCallCount++
	if m.submitShouldFail != nil {
		return m.submitShouldFail
	}
	m.submittedUserID = userID
	m.submittedRole = role
	m.submittedDocType = docType
	m.submittedPayload = payload
	return nil
}

func TestApplicationCoordinator_PrepareSessionUsesFixedForms(t *testing.T) {
	t.Parallel()

	coord := &applicationCoordinator{
		backend: &mockApplicationBackend{},
		forms:   defaultApplicationForms(),
	}

	session, err := coord.PrepareSession(101, roleStudent, applicationTypeAcademicLeave)
	require.NoError(t, err)
	require.Equal(t, "Академический отпуск", session.FormTitle)
	require.Equal(t, 2, session.StepsCount())
	require.Equal(t, fieldKindFile, session.Fields[0].Kind)
	require.Equal(t, fieldKindText, session.Fields[1].Kind)

	emptySession, err := coord.PrepareSession(55, roleTeacher, applicationTypeWorkCertificate)
	require.NoError(t, err)
	require.Equal(t, 0, emptySession.StepsCount())
}

func TestApplicationCoordinator_SubmitDelegatesToBackend(t *testing.T) {
	t.Parallel()

	backend := &mockApplicationBackend{}
	coord := &applicationCoordinator{
		backend: backend,
		forms:   defaultApplicationForms(),
	}

	data := applicationSessionData{
		Role:   roleStudent,
		Type:   applicationTypeStudyTransfer,
		Values: map[string]string{"target_program": "ФКН"},
	}

	err := coord.Submit(context.Background(), 202, data)
	require.NoError(t, err)
	require.Equal(t, 202, int(backend.submittedUserID))
	require.Equal(t, roleStudent, backend.submittedRole)
	require.Equal(t, applicationTypeStudyTransfer, backend.submittedDocType)
	require.Equal(t, "ФКН", backend.submittedPayload["target_program"])
}
