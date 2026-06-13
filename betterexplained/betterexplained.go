// Package betterexplained is the library behind the be command: the HTTP client,
// request shaping, and the typed data models for BetterExplained.
//
// Data comes from the WordPress REST API v2 at betterexplained.com/wp-json/wp/v2/.
// No key or authentication is needed for public read endpoints.
package betterexplained

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://betterexplained.com"

// DefaultUserAgent identifies the client to BetterExplained.
const DefaultUserAgent = "be/dev (+https://github.com/tamnd/betterexplained-cli)"

// ErrNotFound is returned when a category slug does not exist.
var ErrNotFound = errors.New("not found")

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   defaultBaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client talks to the BetterExplained WordPress API.
type Client struct {
	httpClient *http.Client
	cfg        Config
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client with the given config.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		cfg:        cfg,
	}
}

// Top returns the n most recently published articles.
func (c *Client) Top(ctx context.Context, n int) ([]Article, error) {
	params := url.Values{}
	params.Set("_embed", "1")
	return c.fetchPosts(ctx, params, n)
}

// Search returns up to n articles matching query using the WordPress search API.
func (c *Client) Search(ctx context.Context, query string, n int) ([]Article, error) {
	params := url.Values{}
	params.Set("search", query)
	params.Set("_embed", "1")
	return c.fetchPosts(ctx, params, n)
}

// Categories returns all categories sorted by count descending.
func (c *Client) Categories(ctx context.Context) ([]Category, error) {
	rawURL := c.cfg.BaseURL + "/wp-json/wp/v2/categories?per_page=50"
	var cats []wpCategory
	if err := c.getJSON(ctx, rawURL, &cats); err != nil {
		return nil, err
	}
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].Count > cats[j].Count
	})
	out := make([]Category, len(cats))
	for i, cat := range cats {
		out[i] = Category{
			Rank:  i + 1,
			Name:  cat.Name,
			Slug:  cat.Slug,
			Count: cat.Count,
			URL:   cat.Link,
		}
	}
	return out, nil
}

// Category returns up to n articles in the category with the given slug.
// Returns ErrNotFound if no category matches slug.
func (c *Client) Category(ctx context.Context, slug string, n int) ([]Article, error) {
	rawURL := c.cfg.BaseURL + "/wp-json/wp/v2/categories?per_page=50"
	var cats []wpCategory
	if err := c.getJSON(ctx, rawURL, &cats); err != nil {
		return nil, err
	}
	var catID int
	for _, cat := range cats {
		if cat.Slug == slug {
			catID = cat.ID
			break
		}
	}
	if catID == 0 {
		return nil, fmt.Errorf("category %q: %w", slug, ErrNotFound)
	}
	params := url.Values{}
	params.Set("categories", strconv.Itoa(catID))
	params.Set("_embed", "1")
	return c.fetchPosts(ctx, params, n)
}

// fetchPosts paginates the WP posts endpoint to collect up to n articles.
func (c *Client) fetchPosts(ctx context.Context, params url.Values, n int) ([]Article, error) {
	var out []Article
	page := 1
	for len(out) < n {
		batch := n - len(out)
		if batch > 100 {
			batch = 100
		}
		p := url.Values{}
		for k, vs := range params {
			p[k] = vs
		}
		p.Set("per_page", strconv.Itoa(batch))
		p.Set("page", strconv.Itoa(page))
		rawURL := c.cfg.BaseURL + "/wp-json/wp/v2/posts?" + p.Encode()
		var posts []wpPost
		if err := c.getJSON(ctx, rawURL, &posts); err != nil {
			return out, err
		}
		for _, post := range posts {
			out = append(out, postToArticle(post, len(out)+1))
		}
		if len(posts) < batch {
			break
		}
		page++
	}
	return out, nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
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
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
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

func (c *Client) getJSON(ctx context.Context, rawURL string, v any) error {
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return nil
}

// ─── text helpers ─────────────────────────────────────────────────────────────

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", `"`)
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&apos;", "'")
	out = strings.ReplaceAll(out, "&hellip;", "...")
	return out
}
