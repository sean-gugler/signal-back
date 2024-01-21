package cmd

import (
	// "bytes"
	"database/sql"
	// "encoding/base64"
	// "encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	// "runtime/debug"
	// "strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	// "github.com/xeals/signal-back/signal"
	"github.com/xeals/signal-back/types"
)

// Format fulfils the `formatdb` subcommand.
var FormatDB = cli.Command{
	Name:               "formatDB",
	Usage:              "Extract messages from the database",
	UsageText:          "Parse and transform messages in the database into other formats.",
	CustomHelpTemplate: SubcommandHelp,
	Flags: append([]cli.Flag{
		&cli.StringFlag{
			Name:  "format, f",
			Usage: "output messages as `FORMAT` (xml, csv, raw)",
			Value: "xml",
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
			err error
		)
		if dbfile := c.Args().Get(0); dbfile == "" {
			return errors.New("must specify a Signal database file")
		} else if db, err = sql.Open("sqlite", dbfile); err != nil {
			return errors.Wrap(err, "cannot open database file")
		}


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

		switch strings.ToLower(c.String("format")) {
		case "csv":
			err = CSV_db(db, strings.ToLower(c.String("message")), out)
		case "xml":
			err = XML_db(db, out)
		case "json":
			// err = JSON_db(db, out)
			return errors.New("JSON is still TODO")
		default:
			return errors.Errorf("format %s not recognised", c.String("format"))
		}
		if err != nil {
			return errors.Wrap(err, "failed to format output")
		}

		return nil
	},
}

// JSON <undefined>
func JSON_db(db *sql.DB, out io.Writer) error {
	return nil
}

/*
func csvHeaders(body string) []string {
	cols := strings.Split(body, ", ")
	h := make([]string, 0, len(cols))
	for _, s := range cols {
		h = append(h, s[:strings.IndexRune(s, ' ')])
	}
	return h
}
*/

// CSV dumps the raw backup data into a comma-separated value format.
func CSV_db(db *sql.DB, message string, out io.Writer) error {
	if db == nil || message == "" {
		return nil
	}
	return nil
/*
	ss := make([][]string, 0)
	recipients := map[uint64]types.Recipient{}

	var (
		addressFieldIndex int
		headers []string
		create = fmt.Sprintf(`CREATE TABLE "%s" (`, message)
		insert = "INSERT INTO " + message
	)

	fns := types.ConsumeFuncs{
		StatementFunc: func(s *signal.SqlStatement) error {
			stmt := *s.Statement
			switch {
			case strings.HasPrefix(stmt, create):
				headers = csvHeaders(stmt[ len(create) : len(stmt)-1 ])
				for i, field := range headers {
					if field == "address" {
						addressFieldIndex = i
						break
					}
				}
			case strings.HasPrefix(stmt, "INSERT INTO recipient"):
				id, recipient, err := types.NewRecipientFromStatement(s)
				if err != nil {
					return errors.Wrap(err, "recipient statement couldn't be generated")
				}
				recipients[id] = *recipient
			case strings.HasPrefix(stmt, insert):
				ss = append(ss, types.StatementToStringArray(s))
			}
			return nil
		},
	}

	for id, line := range ss {
		recipientID, err := strconv.ParseUint(line[addressFieldIndex], 10, 64)
		if err != nil {
			panic(err)
		}
		phone := recipients[recipientID].Phone

		ss[id][addressFieldIndex] = phone
	}

	w := csv.NewWriter(out)
	if err := w.Write(headers); err != nil {
		return errors.Wrap(err, "unable to write CSV headers")
	}

	for _, sms := range ss {
		if err := w.Write(sms); err != nil {
			return errors.Wrap(err, "unable to format CSV")
		}
	}

	w.Flush()
	return errors.WithMessage(w.Error(), "unable to end CSV writer or something")
*/
}

// XML formats the backup into the same XML format as SMS Backup & Restore
// uses. Layout described at their website
// http://synctech.com.au/fields-in-xml-backup-files/
func XML_db(db *sql.DB, out io.Writer) error {
	// recipients := map[uint64]types.Recipient{}
	recipients := map[int64]*Recipient{}
	smses := &types.SMSes{}
/*
	type attachmentDetails struct {
		Size uint64
		Body string
	}

	var attachmentBuffer bytes.Buffer
	attachmentEncoder := base64.NewEncoder(base64.StdEncoding, &attachmentBuffer)
	attachments := map[uint64]attachmentDetails{}
	recipients := map[uint64]types.Recipient{}
	smses := &types.SMSes{}
	mmses := map[uint64]types.MMS{}
	mmsParts := map[uint64][]types.MMSPart{}
*/
	// rows, err := SelectStructFromTable(db, Part{}, "part")

	rows, err := SelectStructFromTable(db, Recipient{}, "recipient")
	if err != nil {
		return errors.Wrap(err, "query test")
	}
	for _, row := range rows {
		r := row.(*Recipient)
		recipients[r.ID] = r
		// xml := types.Recipient{
			// Phone: recipient.Phone
		// }
	}

	// rows, err = SelectStructFromTable(db, MMS{}, "mms")

	rows, err = SelectStructFromTable(db, SMS{}, "sms")
	if err != nil {
		return errors.Wrap(err, "query test")
	}
	for _, row := range rows {
		sms := row.(*SMS)
		recipient := recipients[sms.Address]
		xml := types.SMS{
			Address: stringPtr(recipient.Phone),
			ContactName: stringPtr(recipient.System_display_name),
			// Date: strconv.FormatUint(*sms.Date, 10)
			ReadableDate:  intToTime(&sms.Date),
			Body: stringPtr(sms.Body),
		}
		if xml.ContactName == nil {
			xml.ContactName = stringPtr(recipient.Signal_profile_name)
		}

		smses.SMS = append(smses.SMS, xml)
		// log.Printf("%+v", *xml.Body)
		// break
	}
/*
	for id, mms := range mmses {
		var messageSize uint64
		parts, ok := mmsParts[id]
		if ok {
			for i := 0; i < len(parts); i++ {
				if attachment, ok := attachments[parts[i].UniqueID]; ok {
					messageSize += attachment.Size
					parts[i].Data = &attachment.Body
				}
			}
		}
		if mms.Body != nil && len(*mms.Body) > 0 {
			parts = append(parts, types.MMSPart{
				Seq:   0,
				Ct:    "text/plain",
				Name:  "null",
				ChSet: types.CharsetUTF8,
				Cd:    "null",
				Fn:    "null",
				CID:   "null",
				Cl:    fmt.Sprintf("txt%06d.txt", id),
				CttS:  "null",
				CttT:  "null",
				Text:  *mms.Body,
			})
			messageSize += uint64(len(*mms.Body))
			if len(parts) == 1 {
				mms.TextOnly = 1
			}
		}
		if len(parts) == 0 {
			continue
		}
		mms.Parts = parts
		mms.MSize = &messageSize
		if mms.MType == nil {
			if types.SetMMSMessageType(types.MMSSendReq, &mms) != nil {
				panic("logic error: this should never happen")
			}
			smses.MMS = append(smses.MMS, mms)
			if types.SetMMSMessageType(types.MMSRetrieveConf, &mms) != nil {
				panic("logic error: this should never happen")
			}
		}
		smses.MMS = append(smses.MMS, mms)
	}
	for id, sms := range smses.SMS {
		// range gives us COPIES; need to modify original
		smses.SMS[id].Address = recipients[sms.RecipientID].Phone
	}
	for id, mms := range smses.MMS {
		smses.MMS[id].Address = recipients[mms.RecipientID].Phone
	}
*/

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

type Recipient struct{
	ID			int64
	Phone		sql.NullString
	Group_id				sql.NullString
	System_display_name		sql.NullString
	Signal_profile_name		sql.NullString
	Last_profile_fetch		uint64
}

type SMS struct{
	ID			int64
	Address		int64
	Date		uint64
	Body		sql.NullString
}

func stringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
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

func names(fields []reflect.StructField) []string {
	s := make([]string, 0, len(fields))
	for _, f := range fields {
		if f.Name == "ID" {
			// special case, exported struct members cannot begin with _
			s = append(s, "_id")
		} else {
			s = append(s, strings.ToLower(f.Name))
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
	// fields := cachedFieldNames(typ)
	// q := fmt.Sprintf("SELECT %s FROM %s", fields, table)
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

//TODO: dedupe
func intToTime(n *uint64) *string {
	if n == nil {
		return nil
	}
	unix := time.Unix(int64(*n)/1000, 0)
	t := unix.Format("Jan 02, 2006 3:04:05 PM")
	return &t
}
