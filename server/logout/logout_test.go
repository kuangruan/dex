package logout

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dexidp/dex/connector"
	"github.com/dexidp/dex/server/connectors"
	"github.com/dexidp/dex/server/oauth2"
	"github.com/dexidp/dex/storage"
	"github.com/dexidp/dex/storage/memory"
)

// recordingLogoutConnector implements connector.LogoutCallbackConnector and
// records the connectorData it was invoked with so tests can assert what was
// passed down.
type recordingLogoutConnector struct {
	gotConnectorData []byte
	returnURL        string
}

func (c *recordingLogoutConnector) LogoutURL(_ context.Context, connectorData []byte, _ string) (string, error) {
	c.gotConnectorData = connectorData
	return c.returnURL, nil
}

func (c *recordingLogoutConnector) HandleLogoutCallback(_ context.Context, _ *http.Request) error {
	return nil
}

var _ connector.LogoutCallbackConnector = (*recordingLogoutConnector)(nil)

// newTestHandler returns a Handler over a memory storage holding one stored
// "mock" connector, with rec injected into the connector cache so tests can
// observe what tryUpstreamLogout passes down. The injected entry carries the
// same ResourceVersion as the stored connector, so the cache hands back rec
// instead of re-opening the stored config.
func newTestHandler(t *testing.T, rec *recordingLogoutConnector) (*Handler, storage.Storage) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	st := memory.New(logger)

	const connectorID = "mock"
	require.NoError(t, st.CreateConnector(t.Context(), storage.Connector{
		ID:              connectorID,
		Type:            "mockCallback",
		Name:            "Mock",
		ResourceVersion: "1",
	}))

	cache := connectors.NewCache(st, func(conn storage.Connector) (connector.Connector, error) {
		return nil, fmt.Errorf("unexpected resolve of connector %q", conn.ID)
	})
	cache.Set(connectorID, connectors.Connector{
		Type:            "mockCallback",
		ResourceVersion: "1",
		Connector:       rec,
	})

	return &Handler{
		Storage:    st,
		Logger:     logger,
		Connectors: cache,
		IssuerURL:  oauth2.IssuerURL{URL: url.URL{Scheme: "https", Host: "dex.example.com"}},
	}, st
}

// TestTryUpstreamLogoutUsesSessionConnectorData verifies that the connector data
// stored on the auth session at login — the copy that carries the upstream
// id_token — is what reaches the connector's LogoutURL, for use as
// id_token_hint, and that the logout parameters are parked in the session for
// the callback to read back.
//
// netbird's test compared the session's data against a caller-supplied copy
// from the offline session; the upstream handler no longer takes a
// caller-supplied blob (the session is the only source), so the precedence
// cases collapse into "the session's data is what is passed down".
func TestTryUpstreamLogoutUsesSessionConnectorData(t *testing.T) {
	tests := []struct {
		name            string
		sessionConnData []byte
		wantConnData    []byte
	}{
		{
			name:            "session connector data is handed to the connector",
			sessionConnData: []byte(`{"IDToken":"session-token"}`),
			wantConnData:    []byte(`{"IDToken":"session-token"}`),
		},
		{
			name:            "no connector data passes nil",
			sessionConnData: nil,
			wantConnData:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingLogoutConnector{returnURL: "https://upstream.example.com/logout"}
			h, st := newTestHandler(t, rec)

			ctx := t.Context()
			authSession := storage.AuthSession{
				ID:            storage.NewID(),
				Secret:        storage.NewID(),
				UserID:        "test-user",
				ConnectorID:   "mock",
				CreatedAt:     time.Now(),
				LastActivity:  time.Now(),
				ConnectorData: tc.sessionConnData,
			}
			require.NoError(t, st.CreateAuthSession(ctx, authSession))

			redirectURL, ok := h.tryUpstreamLogout(ctx, &authSession,
				"https://dex.example.com/cb", "state-123", "client-123")
			require.True(t, ok)
			require.Equal(t, "https://upstream.example.com/logout", redirectURL)
			require.Equal(t, tc.wantConnData, rec.gotConnectorData)

			stored, err := st.GetAuthSession(ctx, authSession.ID)
			require.NoError(t, err)
			require.NotNil(t, stored.LogoutState)
			require.Equal(t, "https://dex.example.com/cb", stored.LogoutState.PostLogoutRedirectURI)
			require.Equal(t, "state-123", stored.LogoutState.State)
			require.Equal(t, "client-123", stored.LogoutState.ClientID)
		})
	}
}
