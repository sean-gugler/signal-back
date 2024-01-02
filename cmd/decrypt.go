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

type Column struct {
	Name string
	Type string
}

// https://www.sqlite.org/datatype3.html#type_affinity
//     "The type affinity of a column is the recommended type for data stored
//     in that column. The important idea here is that the type is recommended,
//     not required. Any column can still store any type of data."
func schema(statement_params string) []Column {
	// remove parentheses, then split by commas
	cols := strings.Split(unwrap(statement_params, "()"), ",")

	// Directives like "UNIQUE(field, field)" get split by commas, too.
	// Handle this by skipping opening through closing parentheses.
	inParen := false

	// convert each text description into a Column
	s := make([]Column, 0, len(cols))
	for _, desc := range cols {
		trimmed := strings.TrimSpace(desc)
		parts := strings.SplitN(trimmed, " ", 3)
		// ignore parts[3...], optional tags like "DEFAULT" or "PRIMARY"

		name := parts[0]
		if inParen && strings.Index(name, ")") != -1 {
			inParen = false
			continue
		} else if strings.Index(name, "(") != -1 {
			inParen = true
			continue
		}

		sqlType := "<none>"
		if len(parts) > 1 {
			sqlType = parts[1]
		}

		s = append(s, Column{name, sqlType})
	}
	return s
}

// Convert polymorphic "Parameter" type
// into interface{} type, suitable for variadic expansion
// in db.Exec(statement, params...)
func ParameterInterface(p *signal.SqlStatement_SqlParameter, sqlType string) interface{} {
	if p.StringParameter != nil {
		return p.StringParameter
	} else if p.IntegerParameter != nil {
		signed := int64(*p.IntegerParameter)
		return &signed
	} else if p.DoubleParameter != nil {
		return p.DoubleParameter
	} else if p.BlobParameter != nil {
		return p.BlobParameter
	} else if p.NullParameter != nil {
		switch sqlType {
		case "TEXT":
			return p.StringParameter
		case "INTEGER":
			return p.IntegerParameter
		case "REAL":
			return p.DoubleParameter
		case "BLOB":
			return p.BlobParameter
		case "<none>":
			return nil
		default:	
			log.Printf("UNKNOWN TYPE %v", sqlType)
		}
	}
	return nil
}

func WriteDatabase(bf *types.BackupFile, db *sql.DB) error {
	affinity := make(map[string][]Column)
	section := make(map[string]bool)

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			stmt := *s.Statement
			param := make([]interface{}, len(s.Parameters))

			// Log each new section to give a sense of progress
			a := strings.SplitN(stmt, " ", 4)
			if len(a) >= 3 {
				key := strings.Join(a[:3], " ")
				if _, found := section[key]; !found {
					section[key] = true
					log.Println(stmt)
				}
			} else {
				log.Println(stmt)
			}

			if strings.HasPrefix(stmt, "CREATE TABLE ") {
				table := unwrap(a[2], `""`)
				if strings.HasPrefix(table, "sqlite_") {
					log.Printf("*** Skipping RESERVED table name %s", table)
					return nil
				}

				cols := a[3]
				affinity[table] = schema(cols)

			} else if strings.HasPrefix(stmt, "INSERT INTO ") {
				table := unwrap(a[2], `""`)
				col := affinity[table]
				if len(col) < len(param) {
					msg := "More parameters than declared types in '%s'\n%v\n%v"
					return errors.New(fmt.Sprintf(msg, table, col, s.Parameters))
				}
				for i, v := range s.Parameters {
					param[i] = ParameterInterface(v, col[i].Type)
				}

			} else {
				if len(s.Parameters) > 0 {
					return errors.New(fmt.Sprintf("Unexpected parameters, not an INSERT statement: %v", s.Parameters))
				}
			}

			_, err := db.Exec(stmt, param...)
			if err != nil {
				return errors.Wrap(err, "SQL Exec")
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
