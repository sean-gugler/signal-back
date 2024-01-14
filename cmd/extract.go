package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"time"

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
var stickerInfoFilename = "pack_info.json"

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
			if err := os.MkdirAll(path.Join(basePath, folderAttachment), 0755); err != nil {
				return errors.Wrap(err, "unable to create attachment directory")
			}
		}
		if !c.Bool("avatars") {
			if err := os.MkdirAll(path.Join(basePath, folderAvatar), 0755); err != nil {
				return errors.Wrap(err, "unable to create avatar directory")
			}
		}
		if !c.Bool("stickers") {
			if err := os.MkdirAll(path.Join(basePath, folderSticker), 0755); err != nil {
				return errors.Wrap(err, "unable to create sticker directory")
			}
		}
		if err = ExtractFiles(bf, c, basePath); err != nil {
			return errors.Wrap(err, "failed to extract attachment")
		}

		return nil
	},
}

type attachmentInfo struct {
	msg     uint64
	mime    *string
	size    uint64
	name    *string
}
 
type avatarInfo struct {
	DisplayName *string
	ProfileName *string
	fetchTime   uint64
}
 
type stickerInfo struct {
	Pack_id     string
	Title       string
	Author      string
	sticker_id  uint64
	cover       bool
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

	section     := make(map[string]bool)
	attachments := make(map[uint64]attachmentInfo)
	avatars     := make(map[string]avatarInfo)
	stickers    := make(map[uint64]stickerInfo)

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

				ps := s.GetParameters()
				switch table {
				case "part":
					id := *ps[19].IntegerParameter
					attachments[id] = attachmentInfo{
						msg:    *ps[1].IntegerParameter,
						mime:    ps[3].StringParameter,
						size:   *ps[15].IntegerParameter,
						name:    ps[16].StringParameter,
					}
					msg := fmt.Sprintf("found attachment metadata %v: ", id)
					s := attachments[id].mime
					if s == nil {
						msg += "<nil>"
					} else {
						msg += *s
					}
					msg += "   "
					s = attachments[id].name
					if s == nil {
						msg += "<nil>"
					} else {
						msg += *s
					}
					// log.Println(msg)

				case "recipient":
					id := fmt.Sprintf("%d", *ps[0].IntegerParameter)
					avatars[id] = avatarInfo{
						DisplayName:    ps[17].StringParameter,
						ProfileName:    ps[22].StringParameter,
						fetchTime:     *ps[38].IntegerParameter,
					}

				case "sticker":
					id := *ps[0].IntegerParameter
					stickers[id] = stickerInfo{
						Pack_id:    *ps[1].StringParameter,
						Title:      *ps[3].StringParameter,
						Author:     *ps[4].StringParameter,
						sticker_id: *ps[5].IntegerParameter,
						cover:     (*ps[6].IntegerParameter != 0),
					}
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
			id := *a.AttachmentId

			// Write the file without extension, will rename later
			fileName := fmt.Sprintf("%v", id)
			pathName := path.Join(base, folderAttachment, fileName)
			file, err := os.OpenFile(pathName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)

			if err != nil {
				return errors.Wrap(err, "failed to open attachment file")
			}
			defer file.Close()
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt attachment")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close attachment file")
			}

//TODO
//prefs
//sanity check lengths
//timestamp attachment
//use attachment's original filename
//sanity check original extension
//refactor to make all 3 save directly to filename, then rename ext based on mime

			// Report any issues with declared type
			mime := ""
			mimeExt, hasExt := "", false
			info, hasInfo := attachments[id]
			if !hasInfo {
				log.Printf("attachment `%v` has no associated SQL entry", id)
			} else {
				if info.mime == nil {
					log.Printf("file `%v` has no declared MIME type", id)
				} else {
					mime = *info.mime
					mimeExt, hasExt = GetExtension(mime)
					if !hasExt {
						log.Printf("mime type `%s` not recognised", mime)
					}
				}
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
					log.Printf("using declared MIME type: %s (.%s)", mime, mimeExt)
					ext = mimeExt
				} else {
					log.Println("*** Please create a PR or issue if you think it have should been.")
					log.Printf("*** If you can provide details on the file `%v` as well, it would be appreciated", pathName)
				}
			} else {
				if mime != "" && hasExt && (kind.Extension != mimeExt || kind.MIME.Value != mime) {
					log.Printf("detected file type: %s (.%s)", kind.MIME.Value, kind.Extension)
					log.Printf("mismatches declared type: %s (.%s)", mime, mimeExt)
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
				return errors.Wrap(err, "failed to open avatar file")
			}
			defer file.Close()
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt avatar")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close avatar file")
			}

			// Look into the file header itself to detect proper extension.
			ext := ""
			kind, err := filetype.MatchFile(pathName)
			if err != nil {
				log.Println(err.Error())
			}
			if kind == filetype.Unknown {
				log.Printf("unable to detect file type of %v", pathName)
			} else {
				ext = "." + kind.Extension
			}

			// Rename the file
			info := avatars[id]
			if info.DisplayName != nil {
				fileName += fmt.Sprintf(" (%s)", *info.DisplayName)
			} else if info.ProfileName != nil {
				fileName += fmt.Sprintf(" (%s)", *info.ProfileName)
			}
			newName := path.Join(base, folderAvatar, fmt.Sprintf("%s%s", fileName, ext))
			if err = os.Rename(pathName, newName); err != nil {
				msg := fmt.Sprintf("unable to rename avatar file to: %s", newName)
				return errors.Wrap(err, msg)
			}
			if info.fetchTime != 0 {
				atime := time.UnixMilli(0) //leave unchanged
				mtime := time.UnixMilli(int64(info.fetchTime))
				if err = os.Chtimes(newName, atime, mtime); err != nil {
					return errors.Wrap(err, "failed to change timestamp of avatar file")
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
				return errors.Wrap(err, "failed to open sticker file")
			}
			defer file.Close()
			if err = bf.DecryptAttachment(a.GetLength(), file); err != nil {
				return errors.Wrap(err, "failed to decrypt sticker")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close sticker file")
			}

			// Look into the file header itself to detect proper extension.
			ext := ""
			kind, err := filetype.MatchFile(pathName)
			if err != nil {
				log.Println(err.Error())
			}
			if kind == filetype.Unknown {
				log.Printf("unable to detect file type of %v", pathName)
			} else {
				ext = "." + kind.Extension
			}

			// Write pack info
			info := stickers[id]
			packPath := path.Join(base, folderSticker, info.Pack_id)
			if err := os.MkdirAll(packPath, 0755); err != nil {
				msg := fmt.Sprintf("unable to create sticker pack directory: %s", packPath)
				return errors.Wrap(err, msg)
			}
			infoPath := path.Join(packPath, stickerInfoFilename)
			file, err = os.OpenFile(infoPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
			if err != nil {
				return errors.Wrap(err, "failed to open sticker pack info file")
			}
			defer file.Close()
			data, err := json.Marshal(info)
			if err != nil {
				return errors.Wrap(err, "failed to format pack info file")
			}
			if _, err = file.Write(data); err != nil {
				return errors.Wrap(err, "failed to write pack info file")
			}
			if err = file.Close(); err != nil {
				return errors.Wrap(err, "failed to close pack info file")
			}

			// Rename the file
			newName := path.Join(packPath, fmt.Sprintf("%d%s", info.sticker_id, ext))
			if err = os.Rename(pathName, newName); err != nil {
				msg := fmt.Sprintf("unable to rename sticker file to: %s", newName)
				return errors.Wrap(err, msg)
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
