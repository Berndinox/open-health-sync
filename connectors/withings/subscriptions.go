// SPDX-License-Identifier: AGPL-3.0-only

package withings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/soma-intelligence/open-health-sync/internal/connector"
)

// notifyPath is the Withings notify API, under the API base.
const notifyPath = "/notify"

// Subscribe registers push notifications for every category this connector
// pulls (measurements, activity, sleep, workouts). callbackURL is
// <OHS_PUBLIC_URL>/inbound/withings; the signed form of it
// (SignedCallbackURL) is what Withings stores. conn.Tokens must hold a valid
// access token and the Withings user id. Subscribing twice is harmless on the
// Withings side.
func (c *Connector) Subscribe(ctx context.Context, conn connector.Connection, callbackURL string) error {
	return c.notifyAll(ctx, conn.Tokens, callbackURL, "subscribe")
}

// Unsubscribe revokes what Subscribe registered.
func (c *Connector) Unsubscribe(ctx context.Context, conn connector.Connection, callbackURL string) error {
	return c.notifyAll(ctx, conn.Tokens, callbackURL, "revoke")
}

func (c *Connector) notifyAll(ctx context.Context, t connector.Tokens, callbackURL, action string) error {
	if t.Access == "" {
		return fmt.Errorf("withings: %s: no access token", action)
	}
	signed, err := c.SignedCallbackURL(callbackURL, t.ProviderUserID)
	if err != nil {
		return err
	}
	for _, appli := range subscribedApplis {
		if err := c.notify(ctx, t.Access, signed, action, appli); err != nil {
			return fmt.Errorf("withings: %s appli %d: %w", action, appli, err)
		}
	}
	return nil
}

// notify sends one notify API request, parameters in the query string.
func (c *Connector) notify(ctx context.Context, token, callbackURL, action string, appli int) error {
	q := url.Values{}
	q.Set("action", action)
	q.Set("callbackurl", callbackURL)
	q.Set("appli", strconv.Itoa(appli))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+notifyPath+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpc.Do(req)
	if err != nil {
		// The URL carries the callback signature: keep it out of the error.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("notify request: %w", err)
	}
	body, err := readBody(resp.Body, 1<<20)
	resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: %w", errUnauthorized, connector.ErrNeedsReauth)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("notify HTTP %d", resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("notify: decode answer: %w", err)
	}
	if env.Status != 0 {
		return fmt.Errorf("notify status %d", env.Status)
	}
	return nil
}
