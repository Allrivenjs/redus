package gcal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/Allrivenjs/redus/internal/store"
)

const apiBase = "https://www.googleapis.com/calendar/v3"

// Client habla con la API de Calendar de una cuenta.
type Client struct {
	http    *http.Client
	Account string
}

func (c *Client) get(ctx context.Context, path string, q url.Values, v any) error {
	u := apiBase + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("calendar %s: %s: %s", path, resp.Status, body)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// PrimaryID devuelve el id del calendario principal, que es el correo de la cuenta.
func (c *Client) PrimaryID(ctx context.Context) (string, error) {
	var cal struct {
		ID string `json:"id"`
	}
	if err := c.get(ctx, "/calendars/primary", nil, &cal); err != nil {
		return "", err
	}
	return cal.ID, nil
}

type apiTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

type apiEvent struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	EventType   string  `json:"eventType"`
	Summary     string  `json:"summary"`
	Location    string  `json:"location"`
	Description string  `json:"description"`
	HTMLLink    string  `json:"htmlLink"`
	HangoutLink string  `json:"hangoutLink"`
	Start       apiTime `json:"start"`
	End         apiTime `json:"end"`
	Conference  *struct {
		EntryPoints []struct {
			Type string `json:"entryPointType"`
			URI  string `json:"uri"`
		} `json:"entryPoints"`
	} `json:"conferenceData"`
	Attendees []struct {
		Self           bool   `json:"self"`
		ResponseStatus string `json:"responseStatus"`
	} `json:"attendees"`
}

// Events trae las reuniones del calendario principal entre from y to.
// Omite canceladas, rechazadas y bloques que no son reuniones (fuera de
// oficina, tiempo de concentración, ubicación de trabajo).
func (c *Client) Events(ctx context.Context, from, to time.Time) ([]store.Event, error) {
	q := url.Values{
		"timeMin":      {from.Format(time.RFC3339)},
		"timeMax":      {to.Format(time.RFC3339)},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
		"maxResults":   {"250"},
	}
	var res []store.Event
	for {
		var page struct {
			Items         []apiEvent `json:"items"`
			NextPageToken string     `json:"nextPageToken"`
		}
		if err := c.get(ctx, "/calendars/primary/events", q, &page); err != nil {
			return nil, err
		}
		for _, a := range page.Items {
			if e, ok := c.convert(a); ok {
				res = append(res, e)
			}
		}
		if page.NextPageToken == "" {
			return res, nil
		}
		q.Set("pageToken", page.NextPageToken)
	}
}

func (c *Client) convert(a apiEvent) (store.Event, bool) {
	if a.Status == "cancelled" {
		return store.Event{}, false
	}
	switch a.EventType {
	case "outOfOffice", "focusTime", "workingLocation", "birthday":
		return store.Event{}, false
	}
	for _, at := range a.Attendees {
		if at.Self && at.ResponseStatus == "declined" {
			return store.Event{}, false
		}
	}
	e := store.Event{
		ID:       a.ID,
		Account:  c.Account,
		Title:    a.Summary,
		HTMLLink: a.HTMLLink,
		Location: a.Location,
		JoinURL:  joinURL(a),
	}
	if e.Title == "" {
		e.Title = "(sin título)"
	}
	var err error
	if a.Start.DateTime != "" {
		e.Start, err = time.Parse(time.RFC3339, a.Start.DateTime)
		if err != nil {
			return store.Event{}, false
		}
		e.End, err = time.Parse(time.RFC3339, a.End.DateTime)
		if err != nil {
			e.End = e.Start.Add(30 * time.Minute)
		}
	} else {
		e.AllDay = true
		e.Start, _ = time.ParseInLocation("2006-01-02", a.Start.Date, time.Local)
		e.End, _ = time.ParseInLocation("2006-01-02", a.End.Date, time.Local)
	}
	return e, true
}

var meetingURL = regexp.MustCompile(`https://[^\s"'<>]*(meet\.google\.com|zoom\.us|teams\.microsoft\.com|teams\.live\.com|whereby\.com|webex\.com)[^\s"'<>]*`)

func joinURL(a apiEvent) string {
	if a.HangoutLink != "" {
		return a.HangoutLink
	}
	if a.Conference != nil {
		for _, ep := range a.Conference.EntryPoints {
			if ep.Type == "video" && ep.URI != "" {
				return ep.URI
			}
		}
	}
	if m := meetingURL.FindString(a.Location); m != "" {
		return m
	}
	return meetingURL.FindString(a.Description)
}
