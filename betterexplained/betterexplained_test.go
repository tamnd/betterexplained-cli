package betterexplained

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestGetSendsUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.get(context.Background(), srv.URL+"/test")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `[]` {
		t.Errorf("body = %q", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClient(cfg)

	start := time.Now()
	_, err := c.get(context.Background(), srv.URL+"/test")
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestTopReturnsArticles(t *testing.T) {
	posts := []map[string]any{
		{
			"id":   1,
			"date": "2024-01-15T10:00:00",
			"slug": "test-article",
			"link": "https://betterexplained.com/articles/test-article/",
			"title": map[string]any{
				"rendered": "An Intuitive Guide to Calculus",
			},
			"excerpt": map[string]any{
				"rendered": "<p>A short description.</p>",
			},
			"_embedded": map[string]any{
				"author":  []any{map[string]any{"name": "Kalid Azad"}},
				"wp:term": []any{[]any{map[string]any{"name": "Math", "slug": "math"}}},
			},
		},
		{
			"id":   2,
			"date": "2024-02-10T08:00:00",
			"slug": "programming-insight",
			"link": "https://betterexplained.com/articles/programming-insight/",
			"title": map[string]any{
				"rendered": "A Better Way to Learn Programming",
			},
			"excerpt": map[string]any{
				"rendered": "<p>Programming insight here.</p>",
			},
			"_embedded": map[string]any{
				"author":  []any{map[string]any{"name": "Kalid Azad"}},
				"wp:term": []any{[]any{map[string]any{"name": "Programming", "slug": "programming"}}},
			},
		},
	}

	body, _ := json.Marshal(posts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	articles, err := c.Top(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(articles))
	}
	if articles[0].Title != "An Intuitive Guide to Calculus" {
		t.Errorf("articles[0].Title = %q", articles[0].Title)
	}
	if articles[0].Category != "Math" {
		t.Errorf("articles[0].Category = %q, want Math", articles[0].Category)
	}
	if articles[0].Published != "2024-01-15" {
		t.Errorf("articles[0].Published = %q, want 2024-01-15", articles[0].Published)
	}
	if articles[0].URL != "https://betterexplained.com/articles/test-article/" {
		t.Errorf("articles[0].URL = %q", articles[0].URL)
	}
	if articles[1].Category != "Programming" {
		t.Errorf("articles[1].Category = %q, want Programming", articles[1].Category)
	}
	if articles[0].Rank != 1 || articles[1].Rank != 2 {
		t.Errorf("ranks = %d,%d want 1,2", articles[0].Rank, articles[1].Rank)
	}
}

func TestSearchReturnsArticles(t *testing.T) {
	post := map[string]any{
		"id":   3,
		"date": "2023-05-01T09:00:00",
		"slug": "intuitive-linear-algebra",
		"link": "https://betterexplained.com/articles/intuitive-linear-algebra/",
		"title": map[string]any{
			"rendered": "An Intuitive Guide to Linear Algebra",
		},
		"excerpt": map[string]any{
			"rendered": "<p>Linear algebra made intuitive.</p>",
		},
		"_embedded": map[string]any{
			"author":  []any{map[string]any{"name": "Kalid Azad"}},
			"wp:term": []any{[]any{map[string]any{"name": "Math", "slug": "math"}}},
		},
	}

	body, _ := json.Marshal([]any{post})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		if q != "algebra" {
			t.Errorf("search param = %q, want algebra", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	articles, err := c.Search(context.Background(), "algebra", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("got %d articles, want 1", len(articles))
	}
	if articles[0].Title != "An Intuitive Guide to Linear Algebra" {
		t.Errorf("title = %q", articles[0].Title)
	}
}

func TestCategoriesReturnsSorted(t *testing.T) {
	cats := []map[string]any{
		{"id": 3, "count": 5, "name": "Business", "slug": "business", "link": "https://betterexplained.com/articles/category/business/"},
		{"id": 7, "count": 95, "name": "Math", "slug": "math", "link": "https://betterexplained.com/articles/category/math/"},
		{"id": 13, "count": 21, "name": "Calculus", "slug": "calculus", "link": "https://betterexplained.com/articles/category/math/calculus/"},
	}
	body, _ := json.Marshal(cats)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	categories, err := c.Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 3 {
		t.Fatalf("got %d categories, want 3", len(categories))
	}
	// sorted by count descending: 95, 21, 5
	if categories[0].Count != 95 || categories[1].Count != 21 || categories[2].Count != 5 {
		t.Errorf("counts = %d,%d,%d want 95,21,5", categories[0].Count, categories[1].Count, categories[2].Count)
	}
	if categories[0].Rank != 1 || categories[1].Rank != 2 || categories[2].Rank != 3 {
		t.Errorf("ranks = %d,%d,%d want 1,2,3", categories[0].Rank, categories[1].Rank, categories[2].Rank)
	}
	if categories[0].Name != "Math" {
		t.Errorf("categories[0].Name = %q, want Math", categories[0].Name)
	}
}

func TestCategoryResolvesSlug(t *testing.T) {
	catsBody, _ := json.Marshal([]map[string]any{
		{"id": 7, "count": 95, "name": "Math", "slug": "math", "link": "https://betterexplained.com/articles/category/math/"},
	})
	postBody, _ := json.Marshal([]map[string]any{
		{
			"id":      1,
			"date":    "2024-01-01T00:00:00",
			"slug":    "test",
			"link":    "https://betterexplained.com/articles/test/",
			"title":   map[string]any{"rendered": "Test"},
			"excerpt": map[string]any{"rendered": "<p>Test excerpt.</p>"},
			"_embedded": map[string]any{
				"author":  []any{map[string]any{"name": "Kalid Azad"}},
				"wp:term": []any{[]any{map[string]any{"name": "Math", "slug": "math"}}},
			},
		},
	})

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		calls++
		if r.URL.Path == "/wp-json/wp/v2/categories" {
			_, _ = w.Write(catsBody)
			return
		}
		// posts request
		if r.URL.Query().Get("categories") != "7" {
			t.Errorf("categories param = %q, want 7", r.URL.Query().Get("categories"))
		}
		_, _ = w.Write(postBody)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	articles, err := c.Category(context.Background(), "math", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("got %d articles, want 1", len(articles))
	}
	if calls != 2 {
		t.Errorf("expected 2 requests (categories + posts), got %d", calls)
	}
}

func TestCategoryNotFound(t *testing.T) {
	catsBody, _ := json.Marshal([]map[string]any{
		{"id": 7, "count": 95, "name": "Math", "slug": "math", "link": "https://betterexplained.com/articles/category/math/"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(catsBody)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Category(context.Background(), "nonexistent", 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestStripHTML(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<p>Hello &amp; World</p>", "Hello & World"},
		{"<b>Bold</b> text", "Bold text"},
		{"No tags here", "No tags here"},
		{"&lt;code&gt;", "<code>"},
		{"<p>Line &hellip; continues</p>", "Line ... continues"},
		{"  <p>  trimmed  </p>  ", "trimmed"},
	}
	for _, tc := range cases {
		got := stripHTML(tc.in)
		if got != tc.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
