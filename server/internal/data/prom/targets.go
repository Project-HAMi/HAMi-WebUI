package prom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// WalkActiveTargets uses the configured Prometheus transport without buffering the
// response. Callers must discard any partial aggregation if it returns an error.
func (c *Client) WalkActiveTargets(ctx context.Context, visit func(v1.ActiveTarget)) error {
	u := c.client.URL("/api/v1/targets", nil)
	q := u.Query()
	q.Set("state", "active")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("targets returned HTTP %d", resp.StatusCode)
	}
	return decodeActiveTargets(resp.Body, visit)
}

func decodeActiveTargets(body io.Reader, visit func(v1.ActiveTarget)) error {
	dec := json.NewDecoder(body)
	var status string
	var found bool
	err := readObject(dec, func(field string) error {
		switch field {
		case "status":
			return dec.Decode(&status)
		case "data":
			return readObject(dec, func(field string) error {
				if field != "activeTargets" {
					return dec.Decode(new(json.RawMessage))
				}
				if err := expectDelimiter(dec, '['); err != nil {
					return err
				}
				for dec.More() {
					// Labels and scrape metadata outside these fields are unused by discovery.
					var target struct {
						DiscoveredLabels map[string]string
						ScrapeURL        string
						Health           v1.HealthStatus
					}
					if err := dec.Decode(&target); err != nil {
						return err
					}
					visit(v1.ActiveTarget{DiscoveredLabels: target.DiscoveredLabels, ScrapeURL: target.ScrapeURL, Health: target.Health})
				}
				found = true
				return expectDelimiter(dec, ']')
			})
		default:
			return dec.Decode(new(json.RawMessage))
		}
	})
	if err != nil {
		return err
	}
	if status != "success" || !found {
		return fmt.Errorf("invalid active targets response")
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after targets response")
	}
	return nil
}

func readObject(dec *json.Decoder, readField func(string) error) error {
	if err := expectDelimiter(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		// The decoder enforces string keys inside objects.
		if err := readField(key.(string)); err != nil {
			return err
		}
	}
	return expectDelimiter(dec, '}')
}

func expectDelimiter(dec *json.Decoder, want json.Delim) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected %q in targets response", want)
	}
	return nil
}
