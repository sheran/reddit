package models

import (
	"encoding/json"
	"strings"
	"time"
)

type Thing struct {
	Kind string                 `json:"kind"`
	Data map[string]interface{} `json:"data"`
}

type Data struct {
	After     string  `json:"after"`
	Before    string  `json:"before"`
	Dist      int     `json:"dist"`
	Modhash   string  `json:"modhash"`
	GeoFilter string  `json:"geo_filter"`
	Children  []Thing `json:"children"`
}

type Listing struct {
	Kind string `json:"kind"`
	Data Data   `json:"data"`
}

func (l *Listing) Json() string {
	data, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	return string(data)
}

func (l *Listing) GetFirst() *Thing {
	if len(l.Data.Children) > 0 {
		return &l.Data.Children[0]
	}
	return nil
}

func (l *Listing) GetFirstName() string {
	name := l.GetFirst()
	if name != nil {
		return name.Data["name"].(string)
	}
	return ""

}

func (t *Thing) GetPublishTime() time.Time {
	tm := time.Unix(int64(t.Data["created_utc"].(float64)), 0)
	return tm
}

func (t *Thing) GetURL() string {
	// we check for an amp suffix
	ampSuffixes := []string{"/amp", "/amp/"}
	newUrl := t.Data["url"].(string)

	for _, suffix := range ampSuffixes {
		newUrl = strings.TrimSuffix(newUrl, suffix)
	}
	return newUrl
}

type Post struct {
	Title     string `json:"title"`
	Body      string `json:"text"`
	Subreddit string `json:"sr"`
	ApiType   string `json:"api_type"`
	Kind      string `json:"kind"`
	Extension string `json:"extension"`
}
