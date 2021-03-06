package util

import (
	"net/url"
	"testing"

	"github.com/flimzy/diff"
	"github.com/flimzy/testy"
	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kouch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// borrowed from attachments
func addCommonFlags(flags *pflag.FlagSet) {
	flags.String(kouch.FlagFilename, "", "The attachment filename to fetch. Only necessary if the filename contains slashes, to disambiguate from {id}/{filename}.")
	flags.String(kouch.FlagDocument, "", "The document ID. May be provided with the target in the format {id}/{filename}.")
	flags.String(kouch.FlagDatabase, "", "The database. May be provided with the target in the format /{db}/{id}/{filename}")
	flags.StringP(kouch.FlagRev, kouch.FlagShortRev, "", "Retrieves attachment from document of specified revision.")
}

func TestCommonOpts(t *testing.T) {
	tests := []struct {
		name     string
		addFlags func(*pflag.FlagSet)
		scope    kouch.TargetScope
		conf     *kouch.Config
		args     []string
		expected interface{}
		err      string
		status   int
	}{
		{
			name:   "duplicate filenames--test NewTarget() plumbing",
			scope:  kouch.TargetAttachment,
			args:   []string{"--" + kouch.FlagFilename, "foo.txt", "foo.txt"},
			err:    "Must not use --" + kouch.FlagFilename + " and pass separate filename",
			status: chttp.ExitFailedToInitialize,
		},
		{
			name:  "rev",
			scope: kouch.TargetAttachment,
			args:  []string{"--" + kouch.FlagRev, "xyz", "foo.txt"},
			expected: &kouch.Options{
				Target: &kouch.Target{Filename: "foo.txt"},
				Options: &chttp.Options{
					Query: url.Values{"rev": []string{"xyz"}},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.conf == nil {
				test.conf = &kouch.Config{}
			}
			cmd := &cobra.Command{}
			addFlags := test.addFlags
			if addFlags == nil {
				addFlags = addCommonFlags
			}
			addFlags(cmd.Flags())
			if e := cmd.ParseFlags(test.args); e != nil {
				t.Fatal(e)
			}
			ctx := kouch.GetContext(cmd)
			ctx = kouch.SetConf(ctx, test.conf)
			if flags := cmd.Flags().Args(); len(flags) > 0 {
				ctx = kouch.SetTarget(ctx, flags[0])
			}
			kouch.SetContext(kouch.SetConf(ctx, test.conf), cmd)
			opts, err := CommonOptions(ctx, test.scope, cmd.Flags())
			testy.ExitStatusError(t, test.err, test.status, err)
			if d := diff.Interface(test.expected, opts); d != nil {
				t.Error(d)
			}
		})
	}
}
