package mercadopagopix

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// searchTimeLayout is the date format accepted by /v1/payments/search.
const searchTimeLayout = "2006-01-02T15:04:05.000-07:00"

// SearchParams filters /v1/payments/search. Zero values are omitted.
type SearchParams struct {
	ExternalReference string
	Status            Status
	PaymentMethodID   string // e.g. "pix"
	// Begin and End filter by the date in Range ("date_created" by default;
	// also "date_approved", "date_last_updated", "money_release_date").
	Begin, End time.Time
	Range      string
	// Sort is the field to order by ("date_created" by default) and
	// Criteria the direction ("desc" by default).
	Sort     string
	Criteria string
	Limit    int // page size (default 50)
	Offset   int
	// Extra holds any other filter supported by the API.
	Extra url.Values
}

func (p SearchParams) values() url.Values {
	q := url.Values{}
	for k, v := range p.Extra {
		q[k] = v
	}
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("external_reference", p.ExternalReference)
	set("status", string(p.Status))
	set("payment_method_id", p.PaymentMethodID)
	if !p.Begin.IsZero() || !p.End.IsZero() {
		set("range", orDefault(p.Range, "date_created"))
		if !p.Begin.IsZero() {
			set("begin_date", p.Begin.Format(searchTimeLayout))
		}
		if !p.End.IsZero() {
			set("end_date", p.End.Format(searchTimeLayout))
		}
	}
	set("sort", orDefault(p.Sort, "date_created"))
	set("criteria", orDefault(p.Criteria, "desc"))
	q.Set("limit", strconv.Itoa(orDefaultInt(p.Limit, 50)))
	q.Set("offset", strconv.Itoa(p.Offset))
	return q
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orDefaultInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// Paging describes the position of a search page.
type Paging struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// SearchResult is one page of a payment search.
type SearchResult struct {
	Results []Payment `json:"results"`
	Paging  Paging    `json:"paging"`
}

// SearchPayments returns one page of payments matching params.
func (c *Client) SearchPayments(ctx context.Context, params SearchParams, opts ...CallOption) (*SearchResult, error) {
	var res SearchResult
	err := c.do(ctx, request{method: http.MethodGet, path: "/v1/payments/search", query: params.values(), out: &res, opts: opts})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// AllPayments iterates over every payment matching params, fetching pages
// on demand. Stop early by breaking out of the loop. An error ends the
// iteration after being yielded once.
//
//	for p, err := range client.AllPayments(ctx, mercadopagopix.SearchParams{Status: "approved"}) {
//		if err != nil { return err }
//		fmt.Println(p.ID)
//	}
func (c *Client) AllPayments(ctx context.Context, params SearchParams, opts ...CallOption) iter.Seq2[*Payment, error] {
	return func(yield func(*Payment, error) bool) {
		for {
			page, err := c.SearchPayments(ctx, params, opts...)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.Results {
				if !yield(&page.Results[i], nil) {
					return
				}
			}
			params.Offset += len(page.Results)
			if len(page.Results) == 0 || (page.Paging.Total > 0 && params.Offset >= page.Paging.Total) {
				return
			}
		}
	}
}
