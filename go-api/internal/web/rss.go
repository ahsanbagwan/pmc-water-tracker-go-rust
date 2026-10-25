package web

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"time"

	"pmc/internal/model"
)

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	Description string  `xml:"description"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate,omitempty"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type rssDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Language    string    `xml:"language"`
		Items       []rssItem `xml:"item"`
	} `xml:"channel"`
}

const maxRSSItems = 30

// rss: RSS 2.0 of recent status updates. encoding/xml escapes user-submitted notes correctly.
func (h *handlers) rss(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListLocalities(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	var verified []model.Locality
	for _, l := range rows {
		if l.Verified {
			verified = append(verified, l)
		}
	}
	sort.SliceStable(verified, func(i, j int) bool {
		a, b := verified[i].LastVerified, verified[j].LastVerified
		if a == nil || b == nil {
			return a != nil
		}
		return a.After(b.Time)
	})
	if len(verified) > maxRSSItems {
		verified = verified[:maxRSSItems]
	}

	site := h.SiteURL
	var doc rssDoc
	doc.Version = "2.0"
	doc.Channel.Title = "Where the Water Reaches -- PMC Ward Register"
	doc.Channel.Link = site
	doc.Channel.Description = "Recent water supply status updates across Pune Municipal Corporation localities."
	doc.Channel.Language = "en-in"
	for _, l := range verified {
		desc := fmt.Sprintf("Status updated to %s.", l.Status)
		if l.Notes != nil && *l.Notes != "" {
			desc = *l.Notes
		}
		lv := "null"
		it := rssItem{
			Title:       fmt.Sprintf("%s: %s", l.Name, l.Status),
			Link:        fmt.Sprintf("%s/?locality=%d", site, *l.ID),
			Description: desc,
		}
		if l.LastVerified != nil {
			lv = l.LastVerified.Format("2006-01-02")
			it.PubDate = l.LastVerified.UTC().Format("Mon, 2 Jan 2006 15:04:05 GMT")
		}
		it.GUID = rssGUID{"false", fmt.Sprintf("locality-%d-%s", *l.ID, lv)}
		doc.Channel.Items = append(doc.Channel.Items, it)
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=UTF-8")
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(doc)
	_ = time.Now
}
