// Package quran is the library behind the quran command line:
// the HTTP client, request shaping, and the typed data models for api.quran.com.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package quran

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to api.quran.com. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "quran/dev (+https://github.com/tamnd/quran-cli)"

// Host is the API host this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "api.quran.com"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// TranslatedName is the English (or requested-language) name of a chapter.
type TranslatedName struct {
	LanguageName string `json:"language_name"`
	Name         string `json:"name"`
}

// Chapter is a single chapter (surah) of the Quran.
type Chapter struct {
	ID              int            `kit:"id" json:"id"`
	NameSimple      string         `json:"name_simple"`
	NameArabic      string         `json:"name_arabic"`
	VersesCount     int            `json:"verses_count"`
	RevelationPlace string         `json:"revelation_place"`
	TranslatedName  TranslatedName `json:"translated_name"`
}

// Verse is a single verse (ayah) of the Quran.
type Verse struct {
	ID          int    `kit:"id" json:"id"`
	VerseNumber int    `json:"verse_number"`
	VerseKey    string `json:"verse_key"`
	TextUthmani string `json:"text_uthmani"`
	PageNumber  int    `json:"page_number"`
	JuzNumber   int    `json:"juz_number"`
}

// Client talks to api.quran.com over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 200ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ListChapters returns all 114 chapters of the Quran.
func (c *Client) ListChapters(ctx context.Context) ([]Chapter, error) {
	body, err := c.Get(ctx, BaseURL+"/api/v4/chapters")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Chapters []Chapter `json:"chapters"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode chapters: %w", err)
	}
	return resp.Chapters, nil
}

// GetChapter returns a single chapter by its number (1–114).
func (c *Client) GetChapter(ctx context.Context, id int) (*Chapter, error) {
	body, err := c.Get(ctx, fmt.Sprintf("%s/api/v4/chapters/%d", BaseURL, id))
	if err != nil {
		return nil, err
	}
	var ch Chapter
	if err := json.Unmarshal(body, &ch); err != nil {
		return nil, fmt.Errorf("decode chapter: %w", err)
	}
	return &ch, nil
}

// ListVerses returns up to limit verses from the given chapter.
func (c *Client) ListVerses(ctx context.Context, chapter, limit int) ([]Verse, error) {
	url := fmt.Sprintf("%s/api/v4/verses/by_chapter/%d?language=en&words=false&per_page=%d", BaseURL, chapter, limit)
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Verses []Verse `json:"verses"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode verses: %w", err)
	}
	return resp.Verses, nil
}

// GetVerse returns a single verse by its key (e.g. "1:1" or "2:255").
func (c *Client) GetVerse(ctx context.Context, key string) (*Verse, error) {
	url := fmt.Sprintf("%s/api/v4/verses/by_key/%s?language=en&words=false", BaseURL, key)
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Verse Verse `json:"verse"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode verse: %w", err)
	}
	return &resp.Verse, nil
}
