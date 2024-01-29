package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
)

var filenameDB = "signal.db"
var FolderAttachment = "Attachments"
var FolderAvatar = "Avatars"
var FolderSticker = "Stickers"
var FolderSettings = "Settings"
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
			Name:  "settings",
			Usage: "Skip extracting settings",
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
			if err := os.MkdirAll(path.Join(basePath, FolderAttachment), 0755); err != nil {
				return errors.Wrap(err, "unable to create attachment directory")
			}
		}
		if !c.Bool("avatars") {
			if err := os.MkdirAll(path.Join(basePath, FolderAvatar), 0755); err != nil {
				return errors.Wrap(err, "unable to create avatar directory")
			}
		}
		if !c.Bool("stickers") {
			if err := os.MkdirAll(path.Join(basePath, FolderSticker), 0755); err != nil {
				return errors.Wrap(err, "unable to create sticker directory")
			}
		}
		if !c.Bool("settings") {
			if err := os.MkdirAll(path.Join(basePath, FolderSettings), 0755); err != nil {
				return errors.Wrap(err, "unable to create settings directory")
			}
		}
		if err = ExtractFiles(bf, c, basePath); err != nil {
			return errors.Wrap(err, "failed to extract attachment")
		}

		return nil
	},
}

type attachmentInfo struct {
	msg  int64
	mime *string
	size int64
	name *string
}

type avatarInfo struct {
	DisplayName *string
	ProfileName *string
	fetchTime   int64
}

type stickerInfo struct {
	Pack_id    string
	Title      string
	Author     string
	size       int64
	sticker_id int64
	cover      bool
}

func createDB(fileName string) (db *sql.DB, err error) {
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
		db, err = createDB(path.Join(base, filenameDB))
		if err != nil {
			return err
		}
		defer db.Close()
	}

	var (
		schema      = make(map[string]*types.Schema)
		section     = make(map[string]bool)
		attachments = make(map[int64]attachmentInfo)
		avatars     = make(map[string]avatarInfo)
		stickers    = make(map[int64]stickerInfo)
		prefs       = make(map[string]map[string]interface{})
	)

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			defer func() {
				if r := recover(); r != nil {
					fmt.Println(*s.Statement)
					fmt.Println(s.Parameters)
					panic(r)
				}
			}()

			stmt := s.GetStatement()
			param := make([]interface{}, len(s.Parameters))

			if strings.HasPrefix(stmt, "CREATE TABLE ") {
				a := strings.SplitN(stmt, " ", 4)
				table := types.Unwrap(a[2], `""`)

				if strings.HasPrefix(table, "sqlite_") {
					if !c.Bool("database") {
						log.Printf("*** Skipping RESERVED table name %s", table)
					}
					return nil
				}
				schema[table] = types.NewSchema(a[3])

			} else if strings.HasPrefix(stmt, "INSERT INTO ") {
				a := strings.SplitN(stmt, " ", 4)
				table := types.Unwrap(a[2], `""`)

				if !c.Bool("database") {
					// Log each new section to give a sense of progress
					if _, found := section[table]; !found {
						section[table] = true
						log.Printf("Populating table `%s` ...", table)
					}
				}

				sch := schema[table]
				ps := s.GetParameters()
				switch table {
				case "part":
					id := *sch.Field(ps, "unique_id").(*int64)
					attachments[id] = attachmentInfo{
						msg:    *sch.Field(ps, "mid").(*int64),
						mime:    sch.Field(ps, "ct").(*string),
						size:   *sch.Field(ps, "data_size").(*int64),
						name:    sch.Field(ps, "file_name").(*string),
					}

				case "recipient":
					n_id := *sch.Field(ps, "_id").(*int64)
					s_id := fmt.Sprintf("%d", n_id)
					avatars[s_id] = avatarInfo{
						DisplayName:    sch.Field(ps, "system_display_name").(*string),
						ProfileName:    sch.Field(ps, "signal_profile_name").(*string),
						fetchTime:     *sch.Field(ps, "last_profile_fetch").(*int64),
					}

				case "sticker":
					id := *sch.Field(ps, "_id").(*int64)
					stickers[id] = stickerInfo{
						Pack_id:    *sch.Field(ps, "pack_id").(*string),
						Title:      *sch.Field(ps, "pack_title").(*string),
						Author:     *sch.Field(ps, "pack_author").(*string),
						size:       *sch.Field(ps, "file_length").(*int64),
						sticker_id: *sch.Field(ps, "sticker_id").(*int64),
						cover:     (*sch.Field(ps, "cover").(*int64) != 0),
					}
				}

				// db.Exec cannot know which member of Parameter struct to use
				// so we convert from a uniform array of polymorphic struct
				// into a generic array of concrete types
				param = sch.RowValues(s.Parameters)
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
			id := int64(*a.AttachmentId)
			info, hasInfo := attachments[id]

			fileName := fmt.Sprintf("%v", id)
			mime := ""

			if !hasInfo {
				log.Printf("attachment `%v` has no associated SQL entry", id)
			} else {
				if info.size != int64(a.GetLength()) {
					log.Printf("attachment length (%d) mismatches SQL entry.size (%d)", a.GetLength(), info.size)
				}
				if info.name != nil {
					fileName += "." + *info.name
				}
				if info.mime == nil {
					log.Printf("file `%v` has no declared MIME type", id)
				} else {
					mime = *info.mime
				}
			}

			safeFileName := escapeFileName(fileName)
			pathName := path.Join(base, FolderAttachment, safeFileName)
			if err := writeAttachment(pathName, a.GetLength(), bf); err != nil {
				return errors.Wrap(err, "attachment")
			} else if newName, err := fixFileExtension(pathName, mime); err != nil {
				return errors.Wrap(err, "attachment")
			} else if err := setFileTimestamp(newName, id); err != nil {
				return errors.Wrap(err, "attachment")
			}
			return nil
		}
	}
	if !c.Bool("avatars") {
		fns.AvatarFunc = func(a *signal.Avatar) error {
			id := *a.RecipientId
			info, hasInfo := avatars[id]

			fileName := fmt.Sprintf("%v", id)
			mtime := int64(0)

			if !hasInfo {
				log.Printf("avatar `%v` has no associated SQL entry", id)
			} else {
				if info.DisplayName != nil {
					fileName += fmt.Sprintf(" (%s)", *info.DisplayName)
				} else if info.ProfileName != nil {
					fileName += fmt.Sprintf(" (%s)", *info.ProfileName)
				}
				mtime = info.fetchTime
			}

			pathName := path.Join(base, FolderAvatar, fileName)
			if err := writeAttachment(pathName, a.GetLength(), bf); err != nil {
				return errors.Wrap(err, "avatar")
			} else if newName, err := fixFileExtension(pathName, ""); err != nil {
				return errors.Wrap(err, "avatar")
			} else if err := setFileTimestamp(newName, mtime); err != nil {
				return errors.Wrap(err, "avatar")
			}
			return nil
		}
	}
	if !c.Bool("stickers") {
		fns.StickerFunc = func(a *signal.Sticker) error {
			id := int64(*a.RowId)
			info, hasInfo := stickers[id]

			fileName := fmt.Sprintf("%v", id)
			packPath := path.Join(base, FolderSticker)

			if !hasInfo {
				log.Printf("sticker `%v` has no associated SQL entry", id)
			} else {
				if info.size != int64(a.GetLength()) {
					log.Printf("sticker length (%d) mismatches SQL entry.size (%d)", a.GetLength(), info.size)
				}
				fileName = fmt.Sprintf("%d", info.sticker_id)

				packPath = path.Join(packPath, info.Pack_id)
				if err := os.MkdirAll(packPath, 0755); err != nil {
					msg := fmt.Sprintf("unable to create sticker pack directory: %s", packPath)
					return errors.Wrap(err, msg)
				}

				infoPath := path.Join(packPath, stickerInfoFilename)
				if err := writeJson(infoPath, info); err != nil {
					return errors.Wrap(err, "sticker pack info")
				}
			}

			pathName := path.Join(packPath, fileName)
			if err := writeAttachment(pathName, a.GetLength(), bf); err != nil {
				return errors.Wrap(err, "sticker")
			} else if _, err := fixFileExtension(pathName, ""); err != nil {
				return errors.Wrap(err, "sticker")
			}
			return nil
		}
	}
	if !c.Bool("settings") {
		fns.PreferenceFunc = func(p *signal.SharedPreference) error {
			file := p.GetFile()
			m, exist := prefs[file]
			if !exist {
				m = make(map[string]interface{})
				prefs[file] = m
			}

			key := *p.Key
			if p.GetIsStringSetValue() {
				m[key] = p.GetStringSetValue()
			} else if p.BooleanValue != nil {
				m[key] = p.GetBooleanValue()
			} else {
				m[key] = p.Value
			}

			return nil
		}
		fns.KeyValueFunc = func(kv *signal.KeyValue) error {
			file := "signal"
			m, exist := prefs[file]
			if !exist {
				m = make(map[string]interface{})
				prefs[file] = m
			}

			key := *kv.Key
			if        kv.BooleanValue != nil {
				m[key] = kv.GetBooleanValue()
			} else if kv.FloatValue != nil {
				m[key] = kv.GetFloatValue()
			} else if kv.IntegerValue != nil {
				m[key] = kv.GetIntegerValue()
			} else if kv.LongValue != nil {
				m[key] = kv.GetLongValue()
			} else if kv.StringValue != nil {
				m[key] = kv.GetStringValue()
			} else {
				m[key] = kv.BlobValue
			}

			return nil
		}
	}

	if err := bf.Consume(fns); err != nil {
		return err
	}

	for fileName, kv := range prefs {
		pathName := path.Join(base, FolderSettings, fileName + ".json")
		if err := writeJson(pathName, kv); err != nil {
			return errors.Wrap(err, "settings")
		}
	}

	log.Println("Done!")

	return nil
}

func writeJson(pathName string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		return errors.Wrap(err, "json marshal error")
	}
	return writeFile(pathName, func(file io.Writer) error {
		_, err := file.Write(data)
		return err
	})
}

func writeAttachment(pathName string, length uint32, bf *types.BackupFile) error {
	return writeFile(pathName, func(file io.Writer) error {
		return bf.DecryptAttachment(length, file)
	})
}

func writeFile(pathName string, write func(w io.Writer) error) error {
	file, err := os.OpenFile(pathName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "failed to create " + pathName)
	}
	defer file.Close()
	if err := write(file); err != nil {
		return errors.Wrap(err, "failed to write " + pathName)
	}
	if err = file.Close(); err != nil {
		return errors.Wrap(err, "failed to close " + pathName)
	}
	return nil
}

func setFileTimestamp(pathName string, milliseconds int64) error {
	if milliseconds != 0 {
		atime := time.UnixMilli(0) //leave unchanged
		mtime := time.UnixMilli(milliseconds)

		if err := os.Chtimes(pathName, atime, mtime); err != nil {
			return errors.Wrap(err, "failed to change timestamp of attachment file")
		}
	}
	return nil
}

// Convert illegal filename characters into url-style %XX substrings
func escapeFileName(fileName string) (string) {
	const illegal = `<>:"/\|?*`
	s := ""
	for _, c := range fileName {
		if c < ' ' || strings.IndexRune(illegal, c) >= 0 {
			s += fmt.Sprintf("%%%02X", c)
		} else {
			s += string(c)
		}
	}
	return s
}

func fixFileExtension(pathName string, mimeType string) (string, error) {
	// Set default extension by MIME type
	ext := ""
	if mimeType != "" {
		mimeExt, hasExt := GetExtension(mimeType)
		if hasExt {
			ext = mimeExt
		} else {
			log.Printf("mime type `%s` not recognised", mimeType)
		}
	}

	// Inspect the file data itself to detect proper extension
	if kind, err := filetype.MatchFile(pathName); err != nil {
		log.Println("MatchFile:", err.Error())
	} else {
		if kind != filetype.Unknown {
			if ext != "" && (kind.MIME.Value != mimeType || kind.Extension != ext) {
				log.Printf("detected file type: %s (.%s)", kind.MIME.Value, kind.Extension)
				log.Printf("mismatches declared type: %s (.%s)", mimeType, ext)
			}
			ext = kind.Extension
		} else {
			log.Printf("unable to detect file type of %v", pathName)
			if ext != "" {
				log.Printf("using declared MIME type: %s (.%s)", mimeType, ext)
			} else {
				log.Println("*** Please create a PR or issue if you think it have should been.")
				log.Printf("*** If you can provide details on the file `%v` as well, it would be appreciated", pathName)
			}
		}
	}

	// If existing extension is already correct, do not double-append
	givenExt := path.Ext(pathName)
	if givenExt == ".jpeg" {
		givenExt = ".jpg"
	}
	if givenExt == "." + ext {
		ext = ""
	}

	// Rename the file with proper extension
	newName := pathName
	if ext != "" {
		newName += "." + ext
		if err := os.Rename(pathName, newName); err != nil {
			return "", errors.Wrap(err, "change extension")
		}
	}
	return newName, nil
}

// No simple API like 'GetExtension(mime)' found in https://github.com/h2non/filetype
// This implementation is modeled after filetype.IsMIMESupported
func GetExtension(mime string) (string, bool) {
	found := false
	ext := ""

	filetype.Types.Range(func(k, v interface{}) bool {
		kind := v.(filetype_types.Type)
		//assert k == kind.Extension
		if kind.MIME.Value == mime {
			ext = kind.Extension
			found = true
		}
		return !found //continue Range until found
	})

	return ext, found
}
