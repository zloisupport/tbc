package commands

import (
	"context"
	"os"

	"tbc/internal/terabox"
	"tbc/internal/util"

	"github.com/urfave/cli/v3"
)

const (
	CliName           = "tbc"
	CookieFileOptName = "cookie-file"
	BaseUrlOptName    = "base-url"
)

var app = &cli.Command{
	Name:    CliName,
	Authors: []any{"SHA-5010"},
	Usage:   "TeraBox CLI client\n   Run without arguments to enter interactive mode.",
	CommandNotFound: func(ctx context.Context, c *cli.Command, s string) {
		cli.ShowAppHelpAndExit(c, 3)
	},
	EnableShellCompletion: true,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    CookieFileOptName,
			Usage:   "TeraBox cookie file\n # if not specified, use TERABOX_COOKIE environment variable",
			Aliases: []string{"c"},
		},
		&cli.StringFlag{
			Name:    BaseUrlOptName,
			Usage:   "TeraBox API base URL\n # if not specified, use TERABOX_BASE_URL environment variable (default: https://www.terabox.com)",
			Aliases: []string{"u"},
		},
	},
	Commands: []*cli.Command{
		listCmd,
		moveCmd,
		copyCmd,
		removeCmd,
		mkdirCmd,
		findCmd,
		infoCmd,
		putCmd,
		getCmd,
		dfCmd,
	},
}

func Execute() error {
	return app.Run(context.Background(), os.Args)
}

func setupClient(cmd *cli.Command) (*terabox.Client, error) {
	cookie, err := util.GetCookie(cmd.Root().String(CookieFileOptName))
	if err != nil {
		return nil, err
	}
	return terabox.NewClientWithBaseURL(cookie, cmd.Root().String(BaseUrlOptName), nil)
}
