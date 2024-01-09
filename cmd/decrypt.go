package cmd

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/signal"
	"github.com/xeals/signal-back/types"
	_ "modernc.org/sqlite"
)

// Decrypt fulfills the `decrypt` subcommand.
var Decrypt = cli.Command{
	Name:               "decrypt",
	Usage:              "Decrypt the backup file",
	UsageText:          "Parse and extract the contents of the backup file into a sqlite3 database file.",
	CustomHelpTemplate: SubcommandHelp,
 	Flags: append([]cli.Flag{
		&cli.StringFlag{
			Name:  "output, o",
			Usage: "write decrypted database to `FILE`",
			Value: "backup.db",
		},
	}, coreFlags...),
	Action: func(c *cli.Context) error {
		bf, err := setup(c)
		if err != nil {
			return err
		}

		fileName := c.String("output")
		log.Printf("Begin decrypt into %s", fileName)

		if err = os.Remove(fileName); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "creating fresh database")
		}

		db, err := sql.Open("sqlite", fileName)
		if err != nil {
			return errors.Wrap(err, "cannot create database file")
		}
		defer func() {
			db.Close()
		}()

		return WriteDatabase(bf, db)
	},
}

// Remove delimiters such as () or "" that may wrap a substring
func unwrap(s string, delim string) string {
	if len(s) > 2 && s[0] == delim[0] && s[len(s)-1] == delim[1] {
		return s[1:len(s)-1]
	} else {
		return s
	}
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

func WriteDatabase(bf *types.BackupFile, db *sql.DB) error {
	section := make(map[string]bool)

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			stmt := *s.Statement
			param := make([]interface{}, len(s.Parameters))

			if strings.HasPrefix(stmt, "CREATE TABLE ") {
				a := strings.SplitN(stmt, " ", 4)
				table := unwrap(a[2], `""`)
				if strings.HasPrefix(table, "sqlite_") {
					log.Printf("*** Skipping RESERVED table name %s", table)
					return nil
				}

			} else if strings.HasPrefix(stmt, "INSERT INTO ") {
				// Log each new section to give a sense of progress
				a := strings.SplitN(stmt, " ", 4)
				table := unwrap(a[2], `""`)
				if _, found := section[table]; !found {
					section[table] = true
					log.Printf("Populating table %s ...", table)
				}

				// db.Exec cannot know which member of Parameter struct to use
				// so we convert from a uniform array of polymorphic struct
				// into a generic array of concrete types
				for i, v := range s.Parameters {
					param[i] = ParameterValue(v)
				}
			}

			_, err := db.Exec(stmt, param...)
			if err != nil {
				detail := fmt.Sprintf("%s\n%v\nSQL Exec", stmt, param)
				return errors.Wrap(err, detail)
			}
			return nil
		},
	}

	if err := bf.Consume(fns); err != nil {
		return err
	}

	log.Println("Done!")

	return nil
}
