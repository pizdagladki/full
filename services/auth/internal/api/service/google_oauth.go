package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	googleUserinfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"

	// oauthTimeout caps the total time allowed for the token exchange +
	// userinfo HTTP round-trip combined, guarding against hung endpoints.
	oauthTimeout = 10 * time.Second

	// userinfoBodyLimit caps the userinfo response body read (1 MiB) so a
	// malicious or abnormally large response cannot exhaust memory.
	userinfoBodyLimit = 1 << 20
)

type googleOAuth struct {
	cfg         *oauth2.Config
	userinfoURL string
}

// NewGoogleOAuth returns an OAuthExchanger that exchanges authorization codes
// with Google's OpenID Connect endpoint.
func NewGoogleOAuth(clientID, clientSecret, redirectURL string) OAuthExchanger {
	return newGoogleOAuthWithEndpoints(clientID, clientSecret, redirectURL,
		google.Endpoint.TokenURL, googleUserinfoEndpoint)
}

// newGoogleOAuthWithEndpoints is the internal constructor that accepts
// explicit token and userinfo URLs, enabling tests to inject httptest servers.
func newGoogleOAuthWithEndpoints(clientID, clientSecret, redirectURL, tokenURL, userinfoURL string) *googleOAuth {
	return &googleOAuth{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenURL,
				AuthURL:  google.Endpoint.AuthURL,
			},
			Scopes: []string{"openid", "email", "profile"},
		},
		userinfoURL: userinfoURL,
	}
}

type userinfoResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

func (g *googleOAuth) ExchangeCode(ctx context.Context, code string) (GoogleUser, error) {
	// Fix 2: bound both the exchange and the userinfo GET with a single timeout.
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()

	// Inject a timeout-bounded HTTP client so both cfg.Exchange and
	// cfg.Client use it (oauth2 reads oauth2.HTTPClient from the context).
	// oauth2.HTTPClient is the documented context key for injecting a custom
	// transport; both cfg.Exchange and cfg.Client pick it up automatically.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: oauthTimeout}) //nolint:staticcheck

	token, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return GoogleUser{}, fmt.Errorf("%w: exchange: %w", ErrInvalidCode, err)
	}

	client := g.cfg.Client(ctx, token)

	resp, err := client.Get(g.userinfoURL) //nolint:noctx // client carries ctx via oauth2.Config.Client
	if err != nil {
		return GoogleUser{}, fmt.Errorf("%w: userinfo: %w", ErrInvalidCode, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return GoogleUser{}, fmt.Errorf("%w: userinfo status %d", ErrInvalidCode, resp.StatusCode)
	}

	// Fix 3: cap the body read to avoid exhausting memory on a huge response.
	body, err := io.ReadAll(io.LimitReader(resp.Body, userinfoBodyLimit))
	if err != nil {
		return GoogleUser{}, fmt.Errorf("%w: read userinfo body: %w", ErrInvalidCode, err)
	}

	var info userinfoResponse

	err = json.Unmarshal(body, &info)
	if err != nil {
		return GoogleUser{}, fmt.Errorf("%w: decode userinfo: %w", ErrInvalidCode, err)
	}

	return GoogleUser(info), nil
}
