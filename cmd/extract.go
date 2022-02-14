package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/h2non/filetype"
	filetype_types "github.com/h2non/filetype/types"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/signal"
	"github.com/xeals/signal-back/types"
)

// Extract fulfils the `extract` subcommand.
var Extract = cli.Command{
	Name:               "extract",
	Usage:              "Retrieve attachments from the backup",
	UsageText:          "Decrypt files embedded in the backup.",
	CustomHelpTemplate: SubcommandHelp,
	Flags: append([]cli.Flag{
		&cli.StringFlag{
			Name:  "outdir, o",
			Usage: "output attachments to `DIRECTORY`",
		},
	}, coreFlags...),
	Action: func(c *cli.Context) error {
		bf, err := setup(c)
		if err != nil {
			return err
		}

		if path := c.String("outdir"); path != "" {
			err := os.MkdirAll(path, 0755)
			if err != nil {
				return errors.Wrap(err, "unable to create output directory")
			}
			err = os.Chdir(path)
			if err != nil {
				return errors.Wrap(err, "unable to change working directory")
			}
		}
		if err = ExtractAttachments(bf); err != nil {
			return errors.Wrap(err, "failed to extract attachment")
		}

		return nil
	},
}

// ExtractAttachments pulls only the attachments out of the backup file and
// outputs them in the current working directory.
func ExtractAttachments(bf *types.BackupFile) error {
	aEncs := make(map[uint64]string)
	defer func() {
		if r := recover(); r != nil {
			log.Println("Panicked during extraction:", r)
		}
	}()
	defer bf.Close()

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			stmt := s.GetStatement()
			if strings.HasPrefix(stmt, "INSERT INTO part") {
				ps := s.GetParameters()
				id := *ps[19].IntegerParameter
				mime := *ps[3].StringParameter

				aEncs[id] = mime
				log.Printf("found attachment metadata %v:%v `%v`\n", id, mime, ps)
			}
			return nil
		},
		AttachmentFunc: func(a *signal.Attachment) error {
			log.Printf("found attachment binary %v\n", *a.AttachmentId)
			id := *a.AttachmentId

			// Report any issues with declared type
			mime, hasMime := aEncs[id]
			mimeExt, hasExt := GetExtension(mime)
			if !hasMime {
				log.Printf("file `%v` has no associated SQL entry, no declared MIME type", id)
			} else if !hasExt {
				log.Printf("mime type `%s` not recognised\n", mime)
			}

			// Write the file without extension, will rename later
			fileName := fmt.Sprintf("%v", id)
			file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)

			if err != nil {
				return errors.Wrap(err, "failed to open output file")
			}
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt attachment")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close output file")
			}

			// Look into the file header itself to detect proper extension.
			kind, err := filetype.MatchFile(fileName)
			if err != nil {
				log.Println(err.Error())
			}
			ext := kind.Extension

			// Reconcile any inconsistencies with declared type
			if kind == filetype.Unknown {
				log.Printf("unable to detect file type")
				if hasExt {
					log.Printf("using declared MIME type: %s (.%s)\n", mime, mimeExt)
					ext = mimeExt
				} else {
					log.Println("*** Please create a PR or issue if you think it have should been.")
					log.Printf("*** If you can provide details on the file `%v` as well, it would be appreciated", fileName)
				}
			} else {
				log.Printf("detected file type: %s (.%s)", kind.MIME.Value, kind.Extension)
				if hasMime && hasExt && (kind.Extension != mimeExt || kind.MIME.Value != mime) {
					log.Printf("mismatches declared type: %s (.%s)\n", mime, mimeExt)
				}
			}

			// Rename the file with proper extension
			if ext != "" { 
				if err = os.Rename(fileName, fileName+"."+ext); err != nil {
					return errors.Wrap(err, "unable to rename output file")
				}
			}
			return nil
		},
	}

	return bf.Consume(fns)
}

// No simple API like 'GetExtension(mime)' found in https://github.com/h2non/filetype
// This implementation is modeled after filetype.IsMIMESupported
func GetExtension(mime string)(string, bool) {
	found := false
	ext := ""

	filetype.Types.Range(func(k, v interface{}) bool {
		kind := v.(filetype_types.Type)
		if kind.MIME.Value == mime {
			ext = kind.Extension
			found = true
		}
		return !found  //continue Range until found
	})

	return ext, found
}
