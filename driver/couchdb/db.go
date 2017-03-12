package couchdb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flimzy/kivik/driver/couchdb/chttp"
	"github.com/flimzy/kivik/driver/ouchdb"
)

type db struct {
	*client
	dbName string
}

func (d *db) path(path string, query url.Values) string {
	url, _ := url.Parse(d.dbName + "/" + strings.TrimPrefix(path, "/"))
	if query != nil {
		url.RawQuery = query.Encode()
	}
	return url.String()
}

func optionsToParams(opts map[string]interface{}) (url.Values, error) {
	params := url.Values{}
	for key, i := range opts {
		var values []string
		switch i.(type) {
		case string:
			values = []string{i.(string)}
		case []string:
			values = i.([]string)
		default:
			return nil, fmt.Errorf("Cannot convert type %T to []string", i)
		}
		for _, value := range values {
			params.Add(key, value)
		}
	}
	return params, nil
}

// AllDocsContext returns all of the documents in the database.
func (d *db) AllDocsContext(ctx context.Context, docs interface{}, opts map[string]interface{}) (offset, totalrows int, seq string, err error) {
	resp, err := d.Client.DoReq(ctx, chttp.MethodGet, d.path("_all_docs", nil), nil)
	if err != nil {
		return 0, 0, "", err
	}
	if err = chttp.ResponseError(resp.Response); err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	return ouchdb.AllDocs(resp.Body, docs)
}

// GetContext fetches the requested document.
func (d *db) GetContext(ctx context.Context, docID string, doc interface{}, opts map[string]interface{}) error {
	params, err := optionsToParams(opts)
	if err != nil {
		return err
	}
	_, err = d.Client.DoJSON(ctx, http.MethodGet, d.path(docID, params), &chttp.Options{Accept: "application/json; multipart/mixed"}, doc)
	return err
}

func (d *db) CreateDocContext(ctx context.Context, doc interface{}) (docID, rev string, err error) {
	result := struct {
		ID  string `json:"id"`
		Rev string `json:"rev"`
	}{}
	_, err = d.Client.DoJSON(ctx, chttp.MethodPost, d.dbName, &chttp.Options{JSON: doc}, &result)
	return result.ID, result.Rev, err
}

func (d *db) PutContext(ctx context.Context, docID string, doc interface{}) (rev string, err error) {
	result := struct {
		Rev string `json:"rev"`
	}{}
	_, err = d.Client.DoJSON(ctx, chttp.MethodPut, d.path(docID, nil), &chttp.Options{JSON: doc}, &result)
	return result.Rev, err
}

func (d *db) DeleteContext(ctx context.Context, docID, rev string) (string, error) {
	query := url.Values{}
	query.Add("rev", rev)
	var result struct {
		Rev string `json:"rev"`
	}
	_, err := d.Client.DoJSON(ctx, chttp.MethodDelete, d.path(docID, query), nil, &result)
	return result.Rev, err
}

func (d *db) FlushContext(ctx context.Context) (time.Time, error) {
	result := struct {
		T int64 `json:"instance_start_time,string"`
	}{}
	_, err := d.Client.DoJSON(ctx, chttp.MethodPost, d.path("/_ensure_full_commit", nil), nil, &result)
	return time.Unix(0, 0).Add(time.Duration(result.T) * time.Microsecond), err
}
