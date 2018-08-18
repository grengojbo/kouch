package attachments

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/flimzy/diff"
	"github.com/flimzy/testy"
	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kouch"
	"github.com/pkg/errors"
)

func TestGetAttachmentOpts(t *testing.T) {
	tests := []struct {
		name     string
		conf     *kouch.Config
		args     []string
		expected interface{}
		err      string
		status   int
	}{
		{
			name:   "duplicate filenames",
			args:   []string{"--" + kouch.FlagFilename, "foo.txt", "foo.txt"},
			err:    "Must not use --" + kouch.FlagFilename + " and pass separate filename",
			status: chttp.ExitFailedToInitialize,
		},
		{
			name: "id from target",
			conf: &kouch.Config{
				DefaultContext: "foo",
				Contexts:       []kouch.NamedContext{{Name: "foo", Context: &kouch.Context{Root: "foo.com"}}},
			},
			args: []string{"123/foo.txt", "--database", "bar"},
			expected: &opts{Target: &kouch.Target{
				Root:     "foo.com",
				Database: "bar",
				Document: "123",
				Filename: "foo.txt",
			}},
		},
		{
			name:   "doc ID provided twice",
			args:   []string{"123/foo.txt", "--" + kouch.FlagDocument, "321"},
			err:    "Must not use --id and pass document ID as part of the target",
			status: chttp.ExitFailedToInitialize,
		},
		{
			name: "db included in target",
			conf: &kouch.Config{
				DefaultContext: "foo",
				Contexts:       []kouch.NamedContext{{Name: "foo", Context: &kouch.Context{Root: "foo.com"}}},
			},
			args: []string{"/foo/123/foo.txt"},
			expected: &opts{Target: &kouch.Target{
				Root:     "foo.com",
				Database: "foo",
				Document: "123",
				Filename: "foo.txt",
			}},
		},
		{
			name:   "db provided twice",
			args:   []string{"/foo/123/foo.txt", "--" + kouch.FlagDatabase, "foo"},
			err:    "Must not use --" + kouch.FlagDatabase + " and pass database as part of the target",
			status: chttp.ExitFailedToInitialize,
		},
		{
			name: "full url target",
			args: []string{"http://foo.com/foo/123/foo.txt"},
			expected: &opts{Target: &kouch.Target{
				Root:     "http://foo.com",
				Database: "foo",
				Document: "123",
				Filename: "foo.txt",
			}},
		},
		{
			name: "if none match",
			args: []string{"--" + kouch.FlagIfNoneMatch, "xyz", "foo.txt"},
			expected: &opts{
				Target: &kouch.Target{
					Filename: "foo.txt",
				},
				ifNoneMatch: "xyz",
			},
		},
		{
			name: "rev",
			args: []string{"--" + kouch.FlagRev, "xyz", "foo.txt"},
			expected: &opts{
				Target: &kouch.Target{
					Filename: "foo.txt",
				},
				rev: "xyz",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.conf == nil {
				test.conf = &kouch.Config{}
			}
			cmd := attCmd()
			cmd.ParseFlags(test.args)
			ctx := kouch.GetContext(cmd)
			if flags := cmd.Flags().Args(); len(flags) > 0 {
				ctx = kouch.SetTarget(ctx, flags[0])
			}
			kouch.SetContext(kouch.SetConf(ctx, test.conf), cmd)
			opts, err := getAttachmentOpts(cmd, cmd.Flags().Args())
			testy.ExitStatusError(t, test.err, test.status, err)
			if d := diff.Interface(test.expected, opts); d != nil {
				t.Error(d)
			}
		})
	}
}

func TestGetAttachment(t *testing.T) {
	type gaTest struct {
		name     string
		opts     *opts
		resp     *http.Response
		val      testy.RequestValidator
		expected string
		err      string
		status   int
	}
	tests := []gaTest{
		{
			name:   "validation fails",
			opts:   &opts{Target: &kouch.Target{}},
			err:    "No filename provided",
			status: chttp.ExitFailedToInitialize,
		},
		{
			name: "success",
			opts: &opts{Target: &kouch.Target{Database: "foo", Document: "123", Filename: "foo.txt"}},
			val: func(r *http.Request) error {
				if r.URL.Path != "/foo/123/foo.txt" {
					return errors.Errorf("Unexpected path: %s", r.URL.Path)
				}
				return nil
			},
			resp: &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("Test\ncontent\n")),
			},
			expected: "Test\ncontent\n",
		},
		{
			name: "slashes",
			opts: &opts{Target: &kouch.Target{Database: "foo/ba r", Document: "123/b", Filename: "foo/bar.txt"}},
			val: func(r *http.Request) error {
				if r.URL.RawPath != "/foo%2Fba+r/123%2Fb/foo%2Fbar.txt" {
					return errors.Errorf("Unexpected path: %s", r.URL.RawPath)
				}
				return nil
			},
			resp: &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("Test\ncontent\n")),
			},
			expected: "Test\ncontent\n",
		},
		{
			name: "if-none-match",
			opts: &opts{Target: &kouch.Target{Database: "foo/ba r", Document: "123/b", Filename: "foo/bar.txt"}, ifNoneMatch: "xyz"},
			val: func(r *http.Request) error {
				if r.URL.RawPath != "/foo%2Fba+r/123%2Fb/foo%2Fbar.txt" {
					err := errors.Errorf("Unexpected path: %s", r.URL.Path)
					fmt.Println(err)
					return err
				}
				if inm := r.Header.Get("If-None-Match"); inm != "\"xyz\"" {
					err := errors.Errorf("Unexpected If-None-Match header: %s", inm)
					fmt.Println(err)
					return err
				}
				return nil
			},
			resp: &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("Test\ncontent\n")),
			},
			expected: "Test\ncontent\n",
		},
		{
			name: "rev",
			opts: &opts{Target: &kouch.Target{Database: "foo/ba r", Document: "123/b", Filename: "foo/bar.txt"}, rev: "xyz"},
			val: func(r *http.Request) error {
				if r.URL.RawPath != "/foo%2Fba+r/123%2Fb/foo%2Fbar.txt" {
					err := errors.Errorf("Unexpected path: %s", r.URL.Path)
					fmt.Println(err)
					return err
				}
				if rev := r.URL.Query().Get("rev"); rev != "xyz" {
					err := errors.Errorf("Unexpected revision: %s", rev)
					fmt.Println(err)
					return err
				}
				return nil
			},
			resp: &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("Test\ncontent\n")),
			},
			expected: "Test\ncontent\n",
		},
	}
	for _, test := range tests {
		func(test gaTest) {
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()
				if test.resp != nil {
					if test.val != nil {
						s := testy.ServeResponseValidator(test.resp, test.val)
						defer s.Close()
						test.opts.Root = s.URL
					} else {
						s := testy.ServeResponse(test.resp)
						defer s.Close()
						test.opts.Root = s.URL
					}
				}
				result, err := getAttachment(test.opts)
				testy.ExitStatusError(t, test.err, test.status, err)
				defer result.Close()
				content, err := ioutil.ReadAll(result)
				if err != nil {
					t.Fatal(err)
				}
				if d := diff.Text(test.expected, string(content)); d != nil {
					t.Error(d)
				}
			})
		}(test)
	}
}
