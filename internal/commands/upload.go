package commands

import (
	"context"
	"fmt"
	"tbc/internal/runner"
	"tbc/internal/terabox"

	"github.com/urfave/cli/v3"
)

const putCmdName = "put"

var putCmd = &cli.Command{
	Name:      putCmdName,
	Usage:     "Upload files to TeraBox",
	UsageText: fmt.Sprintf("%s %s FILE...", CliName, putCmdName),
	Action:    putAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "destination",
			Aliases:     []string{"d"},
			Usage:       "Remote destination directory",
			Value:       ".",
			HideDefault: true,
		},
		&cli.IntFlag{
			Name:    "split",
			Aliases: []string{"s"},
			Usage:   "Upload a file using N connections (Possible: 1-16)",
			Value:   5,
			Validator: func(arg int64) error {
				if arg < 1 || arg > 16 {
					return fmt.Errorf("--split option must be 1-16")
				}
				return nil
			},
		},
		&cli.StringFlag{
			Name:    "chunk-size",
			Aliases: []string{"n"},
			Usage:   "Deprecated: the upload slice size is chosen automatically by the server",
			Value:   "50M",
			Hidden:  true,
		},
	},
	UseShortOptionHandling: true,
}

func putAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		cli.ShowSubcommandHelp(cmd)
		return fmt.Errorf("Invalid arguments")
	}

	// Create TeraBox client
	client, err := setupClient(cmd)
	if err != nil {
		return err
	}

	opts := terabox.UploadOptions{
		MaxConcurrency: cmd.Int("split"),
	}

	return runner.Upload(ctx, client, cmd.Args().Slice(), cmd.String("destination"), &opts)
}
