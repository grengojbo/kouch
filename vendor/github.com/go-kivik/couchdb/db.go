package couchdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kivik"
	"github.com/go-kivik/kivik/driver"
	"github.com/go-kivik/kivik/errors"
)

type db struct {
	*client
	dbName string
}

var _ driver.DB = &db{}
var _ driver.MetaGetter = &db{}
var _ driver.AttachmentMetaGetter = &db{}

func (d *db) path(path string) string {
	url, err := url.Parse(d.dbName + "/" + strings.TrimPrefix(path, "/"))
	if err != nil {
		panic("THIS IS A BUG: d.path failed: " + err.Error())
	}
	return url.String()
}

func optionsToParams(opts ...map[string]interface{}) (url.Values, error) {
	params := url.Values{}
	for _, optsSet := range opts {
		for key, i := range optsSet {
			var values []string
			switch v := i.(type) {
			case string:
				values = []string{v}
			case []string:
				values = v
			case bool:
				values = []string{fmt.Sprintf("%t", v)}
			case int, uint, uint8, uint16, uint32, uint64, int8, int16, int32, int64:
				values = []string{fmt.Sprintf("%d", v)}
			default:
				return nil, errors.Statusf(kivik.StatusBadAPICall, "kivik: invalid type %T for options", i)
			}
			for _, value := range values {
				params.Add(key, value)
			}
		}
	}
	return params, nil
}

// rowsQuery performs a query that returns a rows iterator.
func (d *db) rowsQuery(ctx context.Context, path string, opts map[string]interface{}) (driver.Rows, error) {
	query, err := optionsToParams(opts)
	if err != nil {
		return nil, err
	}
	options := &chttp.Options{Query: query}
	method := kivik.MethodGet
	if keys, ok := query["keys"]; ok {
		method = kivik.MethodPost
		options.Body = chttp.EncodeBody(map[string][]string{
			"keys": keys,
		})
		delete(query, "keys")
	}
	resp, err := d.Client.DoReq(ctx, method, d.path(path), options)
	if err != nil {
		return nil, err
	}
	if err = chttp.ResponseError(resp); err != nil {
		return nil, err
	}
	return newRows(resp.Body), nil
}

// AllDocs returns all of the documents in the database.
func (d *db) AllDocs(ctx context.Context, opts map[string]interface{}) (driver.Rows, error) {
	return d.rowsQuery(ctx, "_all_docs", opts)
}

// DesignDocs returns all of the documents in the database.
func (d *db) DesignDocs(ctx context.Context, opts map[string]interface{}) (driver.Rows, error) {
	return d.rowsQuery(ctx, "_design_docs", opts)
}

// LocalDocs returns all of the documents in the database.
func (d *db) LocalDocs(ctx context.Context, opts map[string]interface{}) (driver.Rows, error) {
	return d.rowsQuery(ctx, "_local_docs", opts)
}

// Query queries a view.
func (d *db) Query(ctx context.Context, ddoc, view string, opts map[string]interface{}) (driver.Rows, error) {
	return d.rowsQuery(ctx, fmt.Sprintf("_design/%s/_view/%s", chttp.EncodeDocID(ddoc), chttp.EncodeDocID(view)), opts)
}

// Get fetches the requested document.
func (d *db) Get(ctx context.Context, docID string, options map[string]interface{}) (*driver.Document, error) {
	resp, rev, err := d.get(ctx, http.MethodGet, docID, options)
	if err != nil {
		return nil, err
	}
	ct, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, errors.WrapStatus(kivik.StatusBadResponse, err)
	}
	switch ct {
	case "application/json":
		return &driver.Document{
			Rev:           rev,
			ContentLength: resp.ContentLength,
			Body:          resp.Body,
		}, nil
	case "multipart/related":
		boundary := strings.Trim(params["boundary"], "\"")
		if boundary == "" {
			return nil, errors.Statusf(kivik.StatusBadResponse, "kivik: boundary missing for multipart/related response")
		}
		mpReader := multipart.NewReader(resp.Body, boundary)
		body, err := mpReader.NextPart()
		if err != nil {
			return nil, errors.WrapStatus(kivik.StatusBadResponse, err)
		}
		length := int64(-1)
		if cl, e := strconv.ParseInt(body.Header.Get("Content-Length"), 10, 64); e == nil {
			length = cl
		}

		// TODO: Use a TeeReader here, to avoid slurping the entire body into memory at once
		content, err := ioutil.ReadAll(body)
		if err != nil {
			return nil, errors.WrapStatus(kivik.StatusBadResponse, err)
		}
		var metaDoc struct {
			Attachments map[string]attMeta `json:"_attachments"`
		}
		if err := json.Unmarshal(content, &metaDoc); err != nil {
			return nil, errors.WrapStatus(kivik.StatusBadResponse, err)
		}

		return &driver.Document{
			ContentLength: length,
			Rev:           rev,
			Body:          ioutil.NopCloser(bytes.NewBuffer(content)),
			Attachments: &multipartAttachments{
				content:  resp.Body,
				mpReader: mpReader,
				meta:     metaDoc.Attachments,
			},
		}, nil
	default:
		return nil, errors.Statusf(kivik.StatusBadResponse, "kivik: invalid content type in response: %s", ct)
	}
}

type attMeta struct {
	ContentType string `json:"content_type"`
	Size        *int64 `json:"length"`
	Follows     bool   `json:"follows"`
}

type multipartAttachments struct {
	content  io.ReadCloser
	mpReader *multipart.Reader
	meta     map[string]attMeta
}

var _ driver.Attachments = &multipartAttachments{}

func (a *multipartAttachments) Next(att *driver.Attachment) error {
	part, err := a.mpReader.NextPart()
	switch err {
	case io.EOF:
		return err
	case nil:
		// fall through
	default:
		return errors.WrapStatus(kivik.StatusBadResponse, err)
	}

	disp, dispositionParams, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil {
		return errors.WrapStatus(kivik.StatusBadResponse, errors.Wrap(err, "Content-Disposition"))
	}
	if disp != "attachment" {
		return errors.Statusf(kivik.StatusBadResponse, "Unexpected Content-Disposition: %s", disp)
	}
	filename := dispositionParams["filename"]

	meta := a.meta[filename]
	if !meta.Follows {
		return errors.Statusf(kivik.StatusBadResponse, "File '%s' not in manifest", filename)
	}

	size := int64(-1)
	if meta.Size != nil {
		size = *meta.Size
	} else if cl, e := strconv.ParseInt(part.Header.Get("Content-Length"), 10, 64); e == nil {
		size = cl
	}

	var cType string
	if ctHeader, ok := part.Header["Content-Type"]; ok {
		cType, _, err = mime.ParseMediaType(ctHeader[0])
		if err != nil {
			return errors.WrapStatus(kivik.StatusBadResponse, err)
		}
	} else {
		cType = meta.ContentType
	}

	*att = driver.Attachment{
		Filename:    filename,
		Size:        size,
		ContentType: cType,
		Content:     part,
	}
	return nil
}

func (a *multipartAttachments) Close() error {
	return a.content.Close()
}

// Rev returns the most current rev of the requested document.
func (d *db) GetMeta(ctx context.Context, docID string, options map[string]interface{}) (size int64, rev string, err error) {
	resp, rev, err := d.get(ctx, http.MethodHead, docID, options)
	if err != nil {
		return 0, "", err
	}
	return resp.ContentLength, rev, err
}

func (d *db) get(ctx context.Context, method string, docID string, options map[string]interface{}) (*http.Response, string, error) {
	if docID == "" {
		return nil, "", missingArg("docID")
	}

	inm, err := ifNoneMatch(options)
	if err != nil {
		return nil, "", err
	}

	params, err := optionsToParams(options)
	if err != nil {
		return nil, "", err
	}
	opts := &chttp.Options{
		Accept:      "application/json",
		IfNoneMatch: inm,
		Query:       params,
	}
	resp, err := d.Client.DoReq(ctx, method, d.path(chttp.EncodeDocID(docID)), opts)
	if err != nil {
		return nil, "", err
	}
	if respErr := chttp.ResponseError(resp); respErr != nil {
		return nil, "", respErr
	}
	rev, err := chttp.GetRev(resp)
	return resp, rev, err
}

func (d *db) CreateDoc(ctx context.Context, doc interface{}, options map[string]interface{}) (docID, rev string, err error) {
	result := struct {
		ID  string `json:"id"`
		Rev string `json:"rev"`
	}{}

	fullCommit, err := fullCommit(options)
	if err != nil {
		return "", "", err
	}

	path := d.dbName
	if len(options) > 0 {
		params, e := optionsToParams(options)
		if e != nil {
			return "", "", e
		}
		path += "?" + params.Encode()
	}

	opts := &chttp.Options{
		Body:       chttp.EncodeBody(doc),
		FullCommit: fullCommit,
	}
	_, err = d.Client.DoJSON(ctx, kivik.MethodPost, path, opts, &result)
	return result.ID, result.Rev, err
}

func (d *db) Put(ctx context.Context, docID string, doc interface{}, options map[string]interface{}) (rev string, err error) {
	if docID == "" {
		return "", missingArg("docID")
	}
	fullCommit, err := fullCommit(options)
	if err != nil {
		return "", err
	}
	opts := &chttp.Options{
		Body:       chttp.EncodeBody(doc),
		FullCommit: fullCommit,
	}
	var result struct {
		ID  string `json:"id"`
		Rev string `json:"rev"`
	}
	_, err = d.Client.DoJSON(ctx, kivik.MethodPut, d.path(chttp.EncodeDocID(docID)), opts, &result)
	if err != nil {
		return "", err
	}
	if result.ID != docID {
		// This should never happen; this is mostly for debugging and internal use
		return result.Rev, errors.Statusf(kivik.StatusBadResponse, "modified document ID (%s) does not match that requested (%s)", result.ID, docID)
	}
	return result.Rev, nil
}

func (d *db) Delete(ctx context.Context, docID, rev string, options map[string]interface{}) (string, error) {
	if docID == "" {
		return "", missingArg("docID")
	}
	if rev == "" {
		return "", missingArg("rev")
	}

	fullCommit, err := fullCommit(options)
	if err != nil {
		return "", err
	}

	query, err := optionsToParams(options)
	if err != nil {
		return "", err
	}
	query.Add("rev", rev)
	opts := &chttp.Options{
		FullCommit: fullCommit,
		Query:      query,
	}
	resp, err := d.Client.DoReq(ctx, kivik.MethodDelete, d.path(chttp.EncodeDocID(docID)), opts)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() // nolint: errcheck
	return chttp.GetRev(resp)
}

func (d *db) Flush(ctx context.Context) error {
	_, err := d.Client.DoError(ctx, kivik.MethodPost, d.path("/_ensure_full_commit"), nil)
	return err
}

func (d *db) Compact(ctx context.Context) error {
	res, err := d.Client.DoReq(ctx, kivik.MethodPost, d.path("/_compact"), nil)
	if err != nil {
		return err
	}
	return chttp.ResponseError(res)
}

func (d *db) CompactView(ctx context.Context, ddocID string) error {
	if ddocID == "" {
		return missingArg("ddocID")
	}
	res, err := d.Client.DoReq(ctx, kivik.MethodPost, d.path("/_compact/"+ddocID), nil)
	if err != nil {
		return err
	}
	return chttp.ResponseError(res)
}

func (d *db) ViewCleanup(ctx context.Context) error {
	res, err := d.Client.DoReq(ctx, kivik.MethodPost, d.path("/_view_cleanup"), nil)
	if err != nil {
		return err
	}
	return chttp.ResponseError(res)
}

func (d *db) Security(ctx context.Context) (*driver.Security, error) {
	var sec *driver.Security
	_, err := d.Client.DoJSON(ctx, kivik.MethodGet, d.path("/_security"), nil, &sec)
	return sec, err
}

func (d *db) SetSecurity(ctx context.Context, security *driver.Security) error {
	opts := &chttp.Options{
		Body: chttp.EncodeBody(security),
	}
	res, err := d.Client.DoReq(ctx, kivik.MethodPut, d.path("/_security"), opts)
	if err != nil {
		return err
	}
	defer res.Body.Close() // nolint: errcheck
	return chttp.ResponseError(res)
}

func (d *db) Copy(ctx context.Context, targetID, sourceID string, options map[string]interface{}) (targetRev string, err error) {
	if sourceID == "" {
		return "", errors.Status(kivik.StatusBadAPICall, "kivik: sourceID required")
	}
	if targetID == "" {
		return "", errors.Status(kivik.StatusBadAPICall, "kivik: targetID required")
	}
	fullCommit, err := fullCommit(options)
	if err != nil {
		return "", err
	}
	params, err := optionsToParams(options)
	if err != nil {
		return "", err
	}
	opts := &chttp.Options{
		FullCommit:  fullCommit,
		Destination: targetID,
		Query:       params,
	}
	resp, err := d.Client.DoReq(ctx, kivik.MethodCopy, d.path(chttp.EncodeDocID(sourceID)), opts)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() // nolint: errcheck
	return chttp.GetRev(resp)
}

func (d *db) Purge(ctx context.Context, docMap map[string][]string) (*driver.PurgeResult, error) {
	result := &driver.PurgeResult{}
	options := &chttp.Options{
		Body: chttp.EncodeBody(docMap),
	}
	_, err := d.Client.DoJSON(ctx, kivik.MethodPost, d.path("_purge"), options, &result)
	return result, err
}
