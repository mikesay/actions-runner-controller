package actions_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/actions/actions-runner-controller/github/actions"
	"github.com/actions/actions-runner-controller/github/actions/testserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitHubAPIRequest(t *testing.T) {
	ctx := context.Background()

	t.Run("uses the right host/path prefix", func(t *testing.T) {
		scenarios := []struct {
			configURL string
			path      string
			expected  string
		}{
			{
				configURL: "https://github.com/org/repo",
				path:      "/app/installations/123/access_tokens",
				expected:  "https://api.github.com/app/installations/123/access_tokens",
			},
			{
				configURL: "https://www.github.com/org/repo",
				path:      "/app/installations/123/access_tokens",
				expected:  "https://api.github.com/app/installations/123/access_tokens",
			},
			{
				configURL: "http://github.localhost/org/repo",
				path:      "/app/installations/123/access_tokens",
				expected:  "http://api.github.localhost/app/installations/123/access_tokens",
			},
			{
				configURL: "https://my-instance.com/org/repo",
				path:      "/app/installations/123/access_tokens",
				expected:  "https://my-instance.com/api/v3/app/installations/123/access_tokens",
			},
			{
				configURL: "http://localhost/org/repo",
				path:      "/app/installations/123/access_tokens",
				expected:  "http://localhost/api/v3/app/installations/123/access_tokens",
			},
		}

		for _, scenario := range scenarios {
			client, err := actions.NewClient(scenario.configURL, nil)
			require.NoError(t, err)

			req, err := client.NewGitHubAPIRequest(ctx, http.MethodGet, scenario.path, nil)
			require.NoError(t, err)
			assert.Equal(t, scenario.expected, req.URL.String())
		}
	})

	t.Run("sets user agent header if present", func(t *testing.T) {
		client, err := actions.NewClient("http://localhost/my-org", nil, actions.WithUserAgent("my-agent"))
		require.NoError(t, err)

		req, err := client.NewGitHubAPIRequest(ctx, http.MethodGet, "/app/installations/123/access_tokens", nil)
		require.NoError(t, err)

		assert.Equal(t, "my-agent", req.Header.Get("User-Agent"))
	})

	t.Run("sets the body we pass", func(t *testing.T) {
		client, err := actions.NewClient("http://localhost/my-org", nil)
		require.NoError(t, err)

		req, err := client.NewGitHubAPIRequest(
			ctx,
			http.MethodGet,
			"/app/installations/123/access_tokens",
			strings.NewReader("the-body"),
		)
		require.NoError(t, err)

		b, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Equal(t, "the-body", string(b))
	})
}

func TestNewActionsServiceRequest(t *testing.T) {
	ctx := context.Background()
	defaultCreds := &actions.ActionsAuth{Token: "token"}

	t.Run("manages authentication", func(t *testing.T) {
		t.Run("client is brand new", func(t *testing.T) {
			token := defaultActionsToken(t)
			server := testserver.New(t, nil, testserver.WithActionsToken(token))

			client, err := actions.NewClient(server.ConfigURLForOrg("my-org"), defaultCreds)
			require.NoError(t, err)

			req, err := client.NewActionsServiceRequest(ctx, http.MethodGet, "my-path", nil)
			require.NoError(t, err)

			assert.Equal(t, "Bearer "+token, req.Header.Get("Authorization"))
		})

		t.Run("admin token is about to expire", func(t *testing.T) {
			newToken := defaultActionsToken(t)
			server := testserver.New(t, nil, testserver.WithActionsToken(newToken))

			client, err := actions.NewClient(server.ConfigURLForOrg("my-org"), defaultCreds)
			require.NoError(t, err)
			client.ActionsServiceAdminToken = "expiring-token"
			client.ActionsServiceAdminTokenExpiresAt = time.Now().Add(59 * time.Second)

			req, err := client.NewActionsServiceRequest(ctx, http.MethodGet, "my-path", nil)
			require.NoError(t, err)

			assert.Equal(t, "Bearer "+newToken, req.Header.Get("Authorization"))
		})

		t.Run("token is currently valid", func(t *testing.T) {
			tokenThatShouldNotBeFetched := defaultActionsToken(t)
			server := testserver.New(t, nil, testserver.WithActionsToken(tokenThatShouldNotBeFetched))

			client, err := actions.NewClient(server.ConfigURLForOrg("my-org"), defaultCreds)
			require.NoError(t, err)
			client.ActionsServiceAdminToken = "healthy-token"
			client.ActionsServiceAdminTokenExpiresAt = time.Now().Add(1 * time.Hour)

			req, err := client.NewActionsServiceRequest(ctx, http.MethodGet, "my-path", nil)
			require.NoError(t, err)

			assert.Equal(t, "Bearer healthy-token", req.Header.Get("Authorization"))
		})
	})

	t.Run("builds the right URL including api version", func(t *testing.T) {
		server := testserver.New(t, nil)

		client, err := actions.NewClient(server.ConfigURLForOrg("my-org"), defaultCreds)
		require.NoError(t, err)

		req, err := client.NewActionsServiceRequest(ctx, http.MethodGet, "/my/path?name=banana", nil)
		require.NoError(t, err)

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		result := req.URL
		assert.Equal(t, serverURL.Host, result.Host)
		assert.Equal(t, "/tenant/123/my/path", result.Path)
		assert.Equal(t, "banana", result.Query().Get("name"))
		assert.Equal(t, "6.0-preview", result.Query().Get("api-version"))
	})

	t.Run("populates header", func(t *testing.T) {
		server := testserver.New(t, nil)

		client, err := actions.NewClient(server.ConfigURLForOrg("my-org"), defaultCreds, actions.WithUserAgent("my-agent"))
		require.NoError(t, err)

		req, err := client.NewActionsServiceRequest(ctx, http.MethodGet, "/my/path", nil)
		require.NoError(t, err)

		assert.Equal(t, "my-agent", req.Header.Get("User-Agent"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	})
}
