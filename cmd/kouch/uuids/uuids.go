package uuids

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kouch"
	"github.com/go-kivik/kouch/cmd/kouch/registry"
)

func init() {
	registry.Register([]string{"get"}, uuidsCmd())
}

func uuidsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uuids",
		Short: "Returns one or more server-generated UUIDs",
		Long: `Returns one or more Universally Unique Identifiers (UUIDs) from the
CouchDB server.`,
		RunE: getUUIDsCmd,
	}
	cmd.Flags().IntP("count", "C", 1, "Number of UUIDs to return")
	return cmd
}

func getUUIDsCmd(cmd *cobra.Command, _ []string) error {
	ctx := kouch.GetContext(cmd)
	count, err := cmd.Flags().GetInt("count")
	if err != nil {
		return err
	}
	defCtx, err := kouch.Conf(ctx).DefaultCtx()
	if err != nil {
		return err
	}
	result, err := getUUIDs(ctx, defCtx.Root, count)
	if err != nil {
		return err
	}
	return kouch.Outputer(ctx).Output(kouch.Output(ctx), result)
}

func getUUIDs(ctx context.Context, url string, count int) (io.ReadCloser, error) {
	c, err := chttp.New(ctx, url)
	if err != nil {
		return nil, err
	}
	res, err := c.DoReq(ctx, http.MethodGet, fmt.Sprintf("/_uuids?count=%d", count), nil)
	if err != nil {
		return nil, err
	}
	if err = chttp.ResponseError(res); err != nil {
		return nil, err
	}
	return res.Body, nil
}
