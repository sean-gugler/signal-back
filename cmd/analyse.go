package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/signal"
	"github.com/xeals/signal-back/types"
)

// Analyse fulfils the `analyse` subcommand.
var Analyse = cli.Command{
	Name:               "analyse",
	Aliases:            []string{"analyze"},
	Usage:              "Report information about the backup file",
	Description:        "Perform integrity check and password validation on the entire file. \nOptionally display statistical information.",
	CustomHelpTemplate: SubcommandHelp,
	ArgsUsage:          "BACKUPFILE",
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:  "summary, s",
			Usage: "Count each type of frame in the file",
		},
		&cli.BoolFlag{
			Name:  "frames, f",
			Usage: "Report header info for every frame",
		},
		&cli.BoolFlag{
			Name:  "body, b",
			Usage: "Show frame body for every frame (very verbose!)",
		},
	}, coreFlags...),
	Action: func(c *cli.Context) error {
		bf, err := setup(c)
		if err != nil {
			return err
		}

		fmt.Println("Analysing...")
		a, err := AnalyseFile(bf, c)
		if err != nil {
			return errors.WithMessage(err, "failed to analyse file")
		}
		fmt.Println("Password valid, file OK")

		if c.Bool("summary") {
			for key, count := range a {
				fmt.Printf("%v: %v\n", key, count)
			}
		}

		log.Println("\nexample part:", len(examples["stmt_insert_into_part"].GetParameters()), examples["stmt_insert_into_part"])

		return nil
	},
}

var examples = map[string]*signal.SqlStatement{}

// AnalyseFile tabulates the frequency of all records in the backup file.
func AnalyseFile(bf *types.BackupFile, c *cli.Context) (map[string]int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Panicked during extraction:", r)
		}
	}()
	defer bf.Close()

	counts := make(map[string]int)
	statementTypes := make(map[string]string)
	var data_sink io.Writer = ioutil.Discard

	for _, caps := range []string{
		"CREATE TABLE ",
		"CREATE VIRTUAL TABLE ",
		"CREATE INDEX ",
		"CREATE UNIQUE INDEX ",
		"CREATE TRIGGER ",
		"DROP TABLE",
		"DROP INDEX",
		// "INSERT INTO ",
	} {
		key := strings.ToLower(caps)
		key = strings.ReplaceAll(key, " ", "_")
		key = "stmt_" + key
		key = key[:len(key)-1]
		statementTypes[caps] = key
	}

	if c.Bool("frames") || c.Bool("body") {
		desc := fmt.Sprintf("%012X: FRAME %d header:<iv:%x, salt:%x>", 0, 0, bf.IV, bf.Salt)
		fmt.Println(desc)
	}
	if c.Bool("summary") {
		fmt.Println("File version", bf.Version)
	}

	ended := 0
	frame_number := 1

	fns := types.ConsumeFuncs{
		FrameFunc:      func(f *signal.BackupFrame, pos int64, frame_length uint32) error {
			if ended == 1 {
				fmt.Println("*** Warning: more frames found after 'end' frame")
				ended++
			}
			desc := fmt.Sprintf("%012X: FRAME %d length %d", pos, frame_number, frame_length)

			if f.GetHeader() != nil {
				hdr := f.GetHeader()
				desc += fmt.Sprintf(" header:<version:%d iv:%x, salt:%x>", hdr.GetVersion(), hdr.GetIv(), hdr.GetSalt())
				counts["header"]++
				if c.Bool("summary") {
					fmt.Println("File version ", hdr.GetVersion())
				}
			}
			if f.GetVersion() != nil {
				desc += fmt.Sprintf(" version:%d", f.GetVersion().GetVersion())
				counts["version"]++
				if c.Bool("summary") {
					fmt.Println("Database", f.GetVersion())
				}
			}
			if f.GetStatement() != nil {
				stmt := f.GetStatement().GetStatement()
				desc += fmt.Sprintf(" stmt:%v", strings.Split(stmt, " ")[0:3])
				// counts["stmt"]++
			}
			if f.GetPreference() != nil {
				desc += fmt.Sprintf(" pref[%s]", f.GetPreference().GetKey())
				counts["pref"]++
			}
			if f.GetKeyValue() != nil {
				desc += fmt.Sprintf(" keyvalue[%v]", f.GetKeyValue().GetKey())
				counts["keyvalue"]++
			}
			if f.GetAttachment() != nil {
				desc += fmt.Sprintf(" attachment[%d]", f.GetAttachment().GetLength())
				counts["attachment"]++
			}
			if f.GetAvatar() != nil {
				desc += fmt.Sprintf(" avatar[%d]", f.GetAvatar().GetLength())
				counts["avatar"]++
			}
			if f.GetSticker() != nil {
				desc += fmt.Sprintf(" sticker[%d]", f.GetSticker().GetLength())
				counts["sticker"]++
			}
			if f.End != nil {
				desc += fmt.Sprintf(" end[%v]", f.GetEnd())
				counts["end"]++
				if f.GetEnd() {
					ended = 1
				}
			}

			if c.Bool("frames") {
				fmt.Println(desc)
			}
			if c.Bool("body") {
				fmt.Printf("%v\n", f)
			}
			frame_number++

			return nil
		},
		AttachmentFunc: func(a *signal.Attachment) error {
			n := a.GetLength()
			counts["bytes_attachment"] += int(n)
			return bf.DecryptAttachment(n, data_sink)
		},
		AvatarFunc:     func(a *signal.Avatar) error {
			n := a.GetLength()
			counts["bytes_avatar"] += int(n)
			return bf.DecryptAttachment(n, data_sink)
		},
		StickerFunc:    func(s *signal.Sticker) error {
			n := s.GetLength()
			counts["bytes_sticker"] += int(n)
			return bf.DecryptAttachment(n, data_sink)
		},
		StatementFunc:  func(s *signal.SqlStatement) error {
			stmt := s.GetStatement()
			found := false
			for prefix, key := range statementTypes {
				if strings.HasPrefix(stmt, prefix) {
					examples[key] = s
					counts[key]++
					found = true
				}
			}
			if !found && strings.HasPrefix(stmt, "INSERT INTO") {
				table := strings.Split(stmt, " ")[2]
				key := "stmt_insert_into_" + table
				examples[key] = s
				counts[key]++
				found = true
			}
			if !found {
				counts["stmt_other"]++
			}
			return nil
		},
	}

	if err := bf.Consume(fns); err != nil {
		return nil, err
	}

	return counts, nil
}
