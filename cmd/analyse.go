package cmd

import (
	"fmt"
	"io"
	// "io/ioutil"
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
	Usage:              "Information about the backup file",
	UsageText:          "Display statistical information about the backup file.",
	Aliases:            []string{"analyze"},
	CustomHelpTemplate: SubcommandHelp,
	Flags:              coreFlags,
	Action: func(c *cli.Context) error {
		bf, err := setup(c)
		if err != nil {
			return err
		}

		a, err := AnalyseTables(bf)
		for key, count := range a {
			fmt.Printf("%v: %v\n", key, count)
		}

		// fmt.Println("\nexample part:", len(examples["stmt_insert_into_part"].GetParameters()), examples["stmt_insert_into_part"])

		return errors.WithMessage(err, "failed to analyse tables")
	},
}

var examples = map[string]*signal.SqlStatement{}

// Remove delimiters such as () or "" that may wrap a substring
func unwrap(s string, delim string) string {
	if len(s) > 2 && s[0] == delim[0] && s[len(s)-1] == delim[1] {
		return s[1:len(s)-1]
	} else {
		return s
	}
}

// AnalyseTables calculates the frequency of all records in the backup file.
func AnalyseTables(bf *types.BackupFile) (map[string]int, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Panicked during extraction:", r)
		}
	}()
	defer bf.Close()

	counts := make(map[string]int)
	statementTypes := make(map[string]string)
	var data_sink io.Writer

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

	fns := types.ConsumeFuncs{
		FrameFunc:      func(f *signal.BackupFrame) error {
			if f.GetHeader() != nil {
				counts["header"]++
			}
			if f.GetVersion() != nil {
				counts["version"]++
			}
			if f.GetPreference() != nil {
				counts["pref"]++
			}
			if f.GetKeyValue() != nil {
				counts["keyvalue"]++
			}
			if f.GetAttachment() != nil {
				counts["attachment"]++
			}
			if f.GetAvatar() != nil {
				counts["avatar"]++
			}
			if f.GetSticker() != nil {
				counts["sticker"]++
			}
			if f.End != nil {
				counts["end"]++
			}
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
