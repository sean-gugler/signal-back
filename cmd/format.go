package cmd

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	// "runtime/debug"
	// "strconv"
	"strings"
	// "time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/types"
	"github.com/xeals/signal-back/types/synctech"
)

// Format fulfils the `format` subcommand.
var Format = cli.Command{
	Name:               "format",
	Usage:              "Extract messages from a signal database",
	UsageText:          "Parse and transform messages in the database into other formats.",
	CustomHelpTemplate: SubcommandHelp,
	Flags: append([]cli.Flag{
		&cli.StringFlag{
			Name:  "format, f",
			Usage: "output messages as `FORMAT` (xml, csv, raw)",
			Value: "xml",
		},
		&cli.StringFlag{
			Name:  "table, t",
			Usage: "for csv|json, choose which table to format (e.g. sms, mms, part)",
			Value: "sms",
		},
		&cli.StringFlag{
			Name:  "output, o",
			Usage: "write formatted data to `FILE`",
		},
	}, coreFlags...),
	Action: func(c *cli.Context) error {
		if c.Bool("verbose") {
			log.SetOutput(os.Stderr)
		} else {
			log.SetOutput(io.Discard)
		}

		var (
			db *sql.DB
			pathBase string
			err error
		)
		if dbfile := c.Args().Get(0); dbfile == "" {
			return errors.New("must specify a Signal database file")
		} else if db, err = sql.Open("sqlite", dbfile); err != nil {
			return errors.Wrap(err, "cannot open database file")
		} else {
			pathBase = filepath.Dir(dbfile)
		}

		pathAttachments := filepath.Join(pathBase, FolderAttachment)

		var out io.Writer
		if c.String("output") != "" {
			var file *os.File
			file, err = os.OpenFile(c.String("output"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			out = io.Writer(file)
			if err != nil {
				return errors.Wrap(err, "unable to open output file")
			}
			defer func() {
				if file.Close() != nil {
					log.Fatalf("unable to close output file: %s", err.Error())
				}
			}()
		} else {
			out = os.Stdout
		}

		table := strings.ToLower(c.String("table"))

		switch strings.ToLower(c.String("format")) {
		case "json":
			err = JSON(db, table, out)
		case "csv":
			err = CSV(db, table, out)
		case "xml":
			err = Synctech(db, pathAttachments, out)
		default:
			return errors.Errorf("format %s not recognised", c.String("format"))
		}
		if err != nil {
			return errors.Wrap(err, "failed to format output")
		}

		return nil
	},
}

// JSON dumps an entire table into a JSON format.
func JSON(db *sql.DB, table string, out io.Writer) error {
	headers, rows, err := SelectEntireTable(db, table)
	if err != nil {
		return errors.Wrap(err, "selecting table")
	}

	n := len(headers)
	records := make([]map[string]interface{}, 0, len(rows))

	for _, row := range rows {
		values := make(map[string]interface{}, n)
		for i, name := range headers {
			values[name] = row[i]
		}
		records = append(records, values)
	}

	jsonEncoder := json.NewEncoder(out)
	jsonEncoder.SetEscapeHTML(false)
	jsonEncoder.SetIndent("", "\t")
	if err := jsonEncoder.Encode(records); err != nil {
		return errors.Wrap(err, "json encode")
	}

	return nil
}

// CSV dumps an entire table into a comma-separated value format.
func CSV(db *sql.DB, table string, out io.Writer) error {
	headers, rowsI, err := SelectEntireTable(db, table)
	if err != nil {
		return errors.Wrap(err, "selecting table")
	}

	w := csv.NewWriter(out)
	if err := w.Write(headers); err != nil {
		return errors.Wrap(err, "unable to write CSV headers")
	}

	rows := StringifyRows(rowsI)
	if err := w.WriteAll(rows); err != nil {
		return errors.Wrap(err, "unable to format CSV")
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return errors.Wrap(err, "writing CSV")
	}

	return nil
}

// Synctech() formats the backup into an XML format compatible with
// SMS Backup & Restore by SyncTech. Layout described at their website
// https://www.synctech.com.au/sms-backup-restore/fields-in-xml-backup-files/
func Synctech(db *sql.DB, pathAttachments string, out io.Writer) error {
	recipients := map[int64]synctech.DbRecipient{}
	smses := &synctech.SMSes{}
	mmses := []synctech.MMS{}
	mmsParts := map[int64][]synctech.MMSPart{} //key: message id

	rows, err := SelectStructFromTable(db, synctech.DbRecipient{}, "recipient")
	if err != nil {
		return errors.Wrap(err, "xml select recipient")
	}
	for _, row := range rows {
		r := row.(*synctech.DbRecipient)
		recipients[r.ID] = *r
	}

	rows, err = SelectStructFromTable(db, synctech.DbSMS{}, "sms")
	if err != nil {
		return errors.Wrap(err, "xml select sms")
	}
	for _, row := range rows {
		sms := row.(*synctech.DbSMS)
		rcp := recipients[sms.Address]
		xml := synctech.NewSMS(*sms, rcp)
		smses.SMS = append(smses.SMS, xml)
	}

	rows, err = SelectStructFromTable(db, synctech.DbMMS{}, "mms")
	if err != nil {
		return errors.Wrap(err, "xml select mms")
	}
	for _, row := range rows {
		mms := row.(*synctech.DbMMS)
		rcp := recipients[mms.Address]
		xml := synctech.NewMMS(*mms, rcp)
		mmses = append(mmses, xml)
	}

	rows, err = SelectStructFromTable(db, synctech.DbPart{}, "part")
	if err != nil {
		return errors.Wrap(err, "xml select part")
	}
	for _, row := range rows {
		r := row.(*synctech.DbPart)
		mid, xml := synctech.NewPart(*r)
		mmsParts[mid] = append(mmsParts[mid], xml)
	}


	for _, mms := range mmses {
		var messageSize uint64
		id := mms.MId
		parts, ok := mmsParts[id]
		if ok {
			for i, part := range parts {
				if size, data, err := ReadAttachment(pathAttachments, part.UniqueId); err != nil {
					if err == os.ErrNotExist {
						log.Printf("No attachment file found with id = %v", id)
					} else {
						return errors.Wrap(err, "read attachment")
					}
				} else {
					if size != part.DataSize {
						log.Printf("attachment (id %v) file size (%v) mismatches declared size (%v)", part.UniqueId, size, part.DataSize)
					}
					messageSize += size
					parts[i].Data = &data
				}
			}
		}
		if mms.Body != nil && len(*mms.Body) > 0 {
			parts = append(parts, synctech.NewPartText(mms))
			messageSize += uint64(len(*mms.Body))
			if len(parts) == 1 {
				mms.TextOnly = 1
			}
		}
		if len(parts) == 0 {
			continue
		}
		mms.PartList.Parts = parts
		mms.MSize = &messageSize
		if mms.MType == nil {
			if synctech.SetMMSMessageType(synctech.MMSSendReq, &mms) != nil {
				panic("logic error: this should never happen")
			}
			smses.MMS = append(smses.MMS, mms)
			if synctech.SetMMSMessageType(synctech.MMSRetrieveConf, &mms) != nil {
				panic("logic error: this should never happen")
			}
		}
		smses.MMS = append(smses.MMS, mms)
	}

	smses.Count = len(smses.SMS)
	x, err := xml.MarshalIndent(smses, "", "  ")
	if err != nil {
		return errors.Wrap(err, "unable to format XML")
	}

	w := types.NewMultiWriter(out)
	w.W([]byte("<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>\n"))
	w.W([]byte("<?xml-stylesheet type=\"text/xsl\" href=\"sms.xsl\" ?>\n"))
	w.W(x)
	return errors.WithMessage(w.Error(), "failed to write out XML")
}

var sqlColumns = make(map[reflect.Type]string)

func cachedFieldNames(typ reflect.Type) string {
	fields, ok := sqlColumns[typ]
	if !ok {
		// Construct and cache query string
		vf := reflect.VisibleFields(typ)
		fields = strings.Join(names(vf), ", ")
		sqlColumns[typ] = fields
	}
	return fields
}

var snakeCase *strings.Replacer

func makeReplacer() *strings.Replacer {
	r := make([]string, 0, 26 * 2)
	for ch := 'a'; ch <= 'z'; ch++ {
		CH := ch - 'a' + 'A'
		r = append(r, string(CH))
		r = append(r, "_" + string(ch))
	}
	return strings.NewReplacer(r...)
}

func names(fields []reflect.StructField) []string {
	if snakeCase == nil {
		snakeCase = makeReplacer()
	}
	s := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Name == "ID" {
			// special case, exported struct members cannot begin with _
			s = append(s, "_id")
		} else {
			s = append(s, snakeCase.Replace(f.Name)[1:])
		}
	}
	return s
}

//TODO: upgrade project to support generics [T any]
func SelectStructFromTable (db *sql.DB, record interface{}, table string) ([]interface{}, error) {
	var result []interface{}

	typ := reflect.TypeOf(record)
	n := typ.NumField()

	// Perform SELECT query
	q := fmt.Sprintf("SELECT %s FROM %s", cachedFieldNames(typ), table)

	rows, err := db.Query(q)
	if err != nil {
		return nil, errors.Wrap(err, q)
	}
	defer rows.Close()

	// Scan rows into new array of same type as 'record'
	for rows.Next() {
		data := reflect.New(typ)
		val := data.Elem()

		I := make([]interface{}, n)
		for i := 0; i < n; i++ {
			I[i] = val.Field(i).Addr().Interface()
		}

		if err = rows.Scan(I...); err != nil {
			return nil, errors.Wrap(err, "scan")
		}

		result = append(result, data.Interface())
	}
	return result, nil
}

func SelectEntireTable (db *sql.DB, table string) (columnNames []string, records [][]interface{}, result error) {
	q := fmt.Sprintf("SELECT * FROM %s", table)

	rows,err := db.Query(q)
	if err != nil {
		return nil, nil, errors.Wrap(err, q)
	}
	defer rows.Close()

	columnNames, err = rows.Columns()
	if err != nil {
		return nil, nil, errors.Wrap(err, q)
	}

	for rows.Next() {
		columns, err := rows.ColumnTypes()
		if err != nil {
			return nil, nil, errors.Wrap(err, q)
		}
		cols := make([]interface{}, 0, len(columns))

		for _, col := range columns {
			var p interface{} = &sql.NullBool{}
			if typ := col.ScanType(); typ != nil {
				p = reflect.New(typ).Interface()
			}
			cols = append(cols, p)
		}
		if err = rows.Scan(cols...); err != nil {
			return nil, nil, errors.Wrap(err, "scan")
		}

		for i, col := range cols {
			if _, ok := col.(*sql.NullBool); ok {
				cols[i] = nil
			}
		}

		records = append(records, cols)
	}

	return columnNames, records, nil
}

func StringifyRows (vrows [][]interface{}) [][]string {
	srows := [][]string{}
	for _, vrow := range vrows {
		// Convert results into strings
		ss := make([]string, 0, len(vrow))
		for _, v := range vrow {
			s := ""
			if v != nil {
				ptr := reflect.ValueOf(v)
				s = fmt.Sprintf("%v", ptr.Elem())
			}
			ss = append(ss, s)
		}

		srows = append(srows, ss)
	}
	return srows
}

func ReadAttachment(folder string, id uint64) (uint64, string, error) {
	pattern := filepath.Join(folder, fmt.Sprintf("%v*", id))
	if matches, err := filepath.Glob(pattern); err != nil {
		return 0, "", errors.Wrap(err, "find attachment file")
	} else if len(matches) == 0 {
		return 0, "", os.ErrNotExist
	} else {
		return readFileAsBase64(matches[0])
	}
}

func readFileAsBase64(pathName string) (uint64, string, error) {
	var buffer bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buffer)
	defer encoder.Close()

	copier := func(file io.Reader) (int64, error) {
		return io.Copy(encoder, file)
	}
	n, err := readFile(pathName, copier)
	if err != nil {
		return 0, "", err
	}
	return uint64(n), buffer.String(), nil
}

func readFile(pathName string, read func(w io.Reader) (int64, error)) (int64, error) {
	file, err := os.OpenFile(pathName, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return 0, errors.Wrap(err, "readFile")
	}
	defer file.Close()
	n, err := read(file)
	if err != nil {
		return 0, errors.Wrap(err, "readFile")
	}
	if err = file.Close(); err != nil {
		return 0, errors.Wrap(err, "readFile")
	}
	return n, nil
}
