package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/h2non/filetype"
	filetype_types "github.com/h2non/filetype/types"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/signal"
	"github.com/xeals/signal-back/types"
	_ "modernc.org/sqlite"
)

var filenameDB = "signal.db"
var folderAttachment = "Attachments"
var folderAvatar = "Avatars"
var folderSticker = "Stickers"

// Extract fulfils the `extract` subcommand.
var Extract = cli.Command{
	Name:               "extract",
	Usage:              "Decrypt contents into individual files",
	UsageText:          "Decrypt the backup and extract all files inside it.",
	CustomHelpTemplate: SubcommandHelp,
	Flags: append([]cli.Flag{
		&cli.StringFlag{
			Name:  "outdir, o",
			Usage: "output files to `DIRECTORY` (default current directory)",
		},
		&cli.BoolFlag{
			Name:  "attachments",
			Usage: "Skip extracting attachments",
		},
		&cli.BoolFlag{
			Name:  "avatars",
			Usage: "Skip extracting avatars",
		},
		&cli.BoolFlag{
			Name:  "stickers",
			Usage: "Skip extracting stickers",
		},
		&cli.BoolFlag{
			Name:  "database",
			Usage: "Skip extracting database",
		},
	}, coreFlags...),
	Action: func(c *cli.Context) error {
		bf, err := setup(c)
		if err != nil {
			return err
		}

		basePath := c.String("outdir")

		if basePath != "" {
			if err := os.MkdirAll(basePath, 0755); err != nil {
				return errors.Wrap(err, "unable to create output directory")
			}
		}
		if !c.Bool("attachments") {
			if err := os.Mkdir(path.Join(basePath, folderAttachment), 0755); err != nil {
				return errors.Wrap(err, "unable to create attachment directory")
			}
		}
		if !c.Bool("avatars") {
			if err := os.Mkdir(path.Join(basePath, folderAvatar), 0755); err != nil {
				return errors.Wrap(err, "unable to create avatar directory")
			}
		}
		if !c.Bool("stickers") {
			if err := os.Mkdir(path.Join(basePath, folderSticker), 0755); err != nil {
				return errors.Wrap(err, "unable to create sticker directory")
			}
		}
		if err = ExtractFiles(bf, c, basePath); err != nil {
			return errors.Wrap(err, "failed to extract attachment")
		}

		return nil
	},
}
 
func createDB (fileName string) (db *sql.DB, err error) {
	log.Printf("Begin decrypt into %s", fileName)

	if err := os.Remove(fileName); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "creating fresh database")
	}

	db, err = sql.Open("sqlite", fileName)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create database file")
	}
	
	return db, nil
}

func ParameterValue(p *signal.SqlStatement_SqlParameter) interface{} {
	if p.StringParameter != nil {
		return p.StringParameter
	} else if p.IntegerParameter != nil {
		signed := int64(*p.IntegerParameter)
		return signed
	} else if p.DoubleParameter != nil {
		return *p.DoubleParameter
	} else if p.BlobParameter != nil {
		return p.BlobParameter
	}
	return nil
}

// ExtractFiles consumes all decrypted data from the backup file and
// dispatches it to an appropriate location.
func ExtractFiles(bf *types.BackupFile, c *cli.Context, base string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Panicked during extraction:", r)
		}
	}()
	defer bf.Close()

	var db *sql.DB
	var err error
	if !c.Bool("database") {
		db, err = createDB (path.Join(base, filenameDB))
		if err != nil { return err }
		defer db.Close()
	}

	section := make(map[string]bool)
	aEncs := make(map[uint64]string)

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			stmt := s.GetStatement()
			param := make([]interface{}, len(s.Parameters))

			if strings.HasPrefix(stmt, "CREATE TABLE ") {
				a := strings.SplitN(stmt, " ", 4)
				table := unwrap(a[2], `""`)
				if !c.Bool("database") && strings.HasPrefix(table, "sqlite_") {
					log.Printf("*** Skipping RESERVED table name %s", table)
					return nil
				}

			} else if strings.HasPrefix(stmt, "INSERT INTO ") {
				a := strings.SplitN(stmt, " ", 4)
				table := unwrap(a[2], `""`)

				// Log each new section to give a sense of progress
				if _, found := section[table]; !found {
					section[table] = true
					if !c.Bool("database") {
						log.Printf("Populating table `%s` ...", table)
					}
				}

				if table == "part" {
					ps := s.GetParameters()
					// msg := *ps[1].StringParameter
					mime := *ps[3].StringParameter
					// size := *ps[15].IntegerParameter
					// name := ps[16].StringParameter
					id := *ps[19].IntegerParameter

					aEncs[id] = mime
					// log.Printf("found attachment metadata %v:%v `%v`\n", id, mime, ps)
				}

				// db.Exec cannot know which member of Parameter struct to use
				// so we convert from a uniform array of polymorphic struct
				// into a generic array of concrete types
				for i, v := range s.Parameters {
					param[i] = ParameterValue(v)
				}
			}

			if !c.Bool("database") {
				_, err := db.Exec(stmt, param...)
				if err != nil {
					detail := fmt.Sprintf("%s\n%v\nSQL Exec", stmt, param)
					return errors.Wrap(err, detail)
				}
			}

			return nil
		},
	}
	if !c.Bool("attachments") {
		fns.AttachmentFunc = func(a *signal.Attachment) error {
			// log.Printf("found attachment binary %v\n", *a.AttachmentId)
			id := *a.AttachmentId

			// Write the file without extension, will rename later
			fileName := fmt.Sprintf("%v", id)
			pathName := path.Join(base, folderAttachment, fileName)
			file, err := os.OpenFile(pathName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)

			if err != nil {
				return errors.Wrap(err, "failed to open output file")
			}
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt attachment")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close output file")
			}

			// Report any issues with declared type
			mime, hasMime := aEncs[id]
			mimeExt, hasExt := GetExtension(mime)
			if !hasMime {
				log.Printf("file `%v` has no associated SQL entry, no declared MIME type", id)
			} else if !hasExt {
				log.Printf("mime type `%s` not recognised\n", mime)
			}

			// Look into the file header itself to detect proper extension.
			kind, err := filetype.MatchFile(pathName)
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
					log.Printf("*** If you can provide details on the file `%v` as well, it would be appreciated", pathName)
				}
			} else {
				if hasMime && hasExt && (kind.Extension != mimeExt || kind.MIME.Value != mime) {
					log.Printf("detected file type: %s (.%s)", kind.MIME.Value, kind.Extension)
					log.Printf("mismatches declared type: %s (.%s)\n", mime, mimeExt)
				}
			}

			// Rename the file with proper extension
			if ext != "" { 
				if err = os.Rename(pathName, pathName+"."+ext); err != nil {
					return errors.Wrap(err, "unable to rename output file")
				}
			}
			return nil
		}
	}
	if !c.Bool("avatars") {
		fns.AvatarFunc = func(a *signal.Avatar) error {
			id := *a.RecipientId

			// Write the file without extension, will rename later
			fileName := fmt.Sprintf("%v", id)
			pathName := path.Join(base, folderAvatar, fileName)
			file, err := os.OpenFile(pathName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)

			if err != nil {
				return errors.Wrap(err, "failed to open output file")
			}
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt avatar")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close output file")
			}

			// Look into the file header itself to detect proper extension.
			kind, err := filetype.MatchFile(pathName)
			if err != nil {
				log.Println(err.Error())
			}
			if kind == filetype.Unknown {
				log.Printf("unable to detect file type of %v", pathName)
			} else {
				// Rename the file with proper extension
				ext := kind.Extension
				if err = os.Rename(pathName, pathName+"."+ext); err != nil {
					return errors.Wrap(err, "unable to rename output file")
				}
			}

			return nil
		}
	}
	if !c.Bool("stickers") {
		fns.StickerFunc = func(a *signal.Sticker) error {
			id := *a.RowId

			// Write the file without extension, will rename later
			fileName := fmt.Sprintf("%v", id)
			pathName := path.Join(base, folderSticker, fileName)
			file, err := os.OpenFile(pathName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)

			if err != nil {
				return errors.Wrap(err, "failed to open output file")
			}
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt sticker")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close output file")
			}

			// Look into the file header itself to detect proper extension.
			kind, err := filetype.MatchFile(pathName)
			if err != nil {
				log.Println(err.Error())
			}
			if kind == filetype.Unknown {
				log.Printf("unable to detect file type of %v", pathName)
			} else {
				// Rename the file with proper extension
				ext := kind.Extension
				if err = os.Rename(pathName, pathName+"."+ext); err != nil {
					return errors.Wrap(err, "unable to rename output file")
				}
			}

			return nil
		}
	}

	if err := bf.Consume(fns); err != nil {
		return err
	}

	log.Println("Done!")

	return nil
}

// No simple API like 'GetExtension(mime)' found in https://github.com/h2non/filetype
// This implementation is modeled after filetype.IsMIMESupported
func GetExtension(mime string)(string, bool) {
	found := false
	ext := ""

	filetype.Types.Range(func(k, v interface{}) bool {
		kind := v.(filetype_types.Type)
		//assert k == kind.Extension
		if kind.MIME.Value == mime {
			ext = kind.Extension
			found = true
		}
		return !found  //continue Range until found
	})

	return ext, found
}
