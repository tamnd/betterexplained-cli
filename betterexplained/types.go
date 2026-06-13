package betterexplained

import "time"

// Article is the record emitted for BetterExplained articles.
type Article struct {
	Rank      int    `json:"rank"`
	Title     string `json:"title"`
	Category  string `json:"category"`
	Published string `json:"published"`
	Excerpt   string `json:"excerpt"`
	URL       string `json:"url"`
}

// Category is the record emitted for article categories.
type Category struct {
	Rank  int    `json:"rank"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Count int    `json:"count"`
	URL   string `json:"url"`
}

// ─── WP API wire types ────────────────────────────────────────────────────────

type wpPost struct {
	ID       int        `json:"id"`
	Date     string     `json:"date"`
	Slug     string     `json:"slug"`
	Link     string     `json:"link"`
	Title    wpRendered `json:"title"`
	Excerpt  wpRendered `json:"excerpt"`
	Embedded wpEmbedded `json:"_embedded"`
}

type wpRendered struct {
	Rendered string `json:"rendered"`
}

type wpEmbedded struct {
	Author []wpAuthor `json:"author"`
	Term   [][]wpTerm `json:"wp:term"`
}

type wpAuthor struct {
	Name string `json:"name"`
}

type wpTerm struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type wpCategory struct {
	ID    int    `json:"id"`
	Count int    `json:"count"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Link  string `json:"link"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func postToArticle(p wpPost, rank int) Article {
	title := stripHTML(p.Title.Rendered)

	excerpt := stripHTML(p.Excerpt.Rendered)
	rs := []rune(excerpt)
	if len(rs) > 150 {
		excerpt = string(rs[:149]) + "..."
	}

	cat := "Math"
	if len(p.Embedded.Term) > 0 && len(p.Embedded.Term[0]) > 0 {
		cat = p.Embedded.Term[0][0].Name
	}

	pub := p.Date
	if t, err := time.Parse("2006-01-02T15:04:05", p.Date); err == nil {
		pub = t.Format("2006-01-02")
	}

	return Article{
		Rank:      rank,
		Title:     title,
		Category:  cat,
		Published: pub,
		Excerpt:   excerpt,
		URL:       p.Link,
	}
}
