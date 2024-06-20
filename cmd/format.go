package cmd

import (
	"bytes"
	"cmp"
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
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/xeals/signal-back/types"
	"github.com/xeals/signal-back/types/message"
)

// Format fulfils the `format` subcommand.
var Format = cli.Command{
	Name:               "format",
	Usage:              "Export messages from a signal database",
	UsageText:          "Parse and transform messages in the database into other formats.\nXML format is compatible with SMS Backup & Restore by SyncTech",
	CustomHelpTemplate: SubcommandHelp,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "output, o",
			Usage: "Write formatted data to `FILE` (default is console)",
		},
		&cli.StringFlag{
			Name:  "format, f",
			Usage: "Output messages as `FORMAT` (xml, csv, json). " +
			       "Default matches --output file extension, or xml if no output file specified.",
		},
		&cli.StringFlag{
			Name:  "table, t",
			Usage: "For csv|json, choose which table to format (e.g. message, sms). " +
			       "Default matches --output file basename, or 'message' if no output file specified.",
		},
		&cli.BoolFlag{
			Name:  "verbose, v",
			Usage: "Enable verbose logging output",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("verbose") {
			log.SetOutput(os.Stderr)
		} else {
			log.SetOutput(io.Discard)
		}

		var (
			db       *sql.DB
			pathBase string
			err      error
			out      io.Writer
		)
		if dbfile := c.Args().Get(0); dbfile == "" {
			return errors.New("must specify a Signal database file")
		} else if db, err = sql.Open("sqlite", dbfile); err != nil {
			return errors.Wrap(err, "cannot open database file")
		} else {
			pathBase = filepath.Dir(dbfile)
		}

		pathAttachments := filepath.Join(pathBase, FolderAttachment)

		output := c.String("output")
		table := strings.ToLower(c.String("table"))
		format := strings.ToLower(c.String("format"))

		if output == "" {
			if format == "" {
				format = "xml"
			} else if table == "" {
				table = "message"
			}
			out = os.Stdout
		} else {
			ext := filepath.Ext(output)
			base := filepath.Base(output)

			// remove extension from base
			base = base[:len(base)-len(ext)]

			if format == "" && len(ext) > 0 {
				format = ext[1:]  //remove '.'
			}
			if table == "" {
				table = base
			}

			var file *os.File
			file, err = os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			out = io.Writer(file)
			if err != nil {
				return errors.Wrap(err, "unable to open output file")
			}
			defer func() {
				if file.Close() != nil {
					log.Fatalf("unable to close output file: %s", err.Error())
				}
			}()
		}

		switch strings.ToLower(format) {
		case "json":
			err = JSON(db, table, out)
		case "csv":
			err = CSV(db, table, out)
		case "xml":
			err = XML(db, pathAttachments, out)
		default:
			return errors.Errorf("format '%s' not recognised", format)
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

// XML puts the messages into a format viewable with a browser.
func XML(db *sql.DB, pathAttachments string, out io.Writer) error {
	var (
		correspondents = make(map[int64]message.DbCorrespondent)
		threads        = make(map[int64]message.DbThread)
		groups         = make(map[int64]message.DbGroup)
		msgAttachments = make(map[int64][]*message.DbAttachment) //key: message id
		msgs           = message.Messages{}
	)

	rows, err := SelectStructFromTable(db, message.DbCorrespondent{}, "recipient")
	if err != nil {
		return errors.Wrap(err, "xml select recipient")
	}
	for _, row := range rows {
		r := row.(*message.DbCorrespondent)
		correspondents[r.ID] = *r
	}

	rows, err = SelectStructFromTable(db, message.DbThread{}, "thread")
	if err != nil {
		return errors.Wrap(err, "xml select thread")
	}
	for _, row := range rows {
		r := row.(*message.DbThread)
		threads[r.ID] = *r
	}

	rows, err = SelectStructFromTable(db, message.DbGroup{}, "groups")
	if err != nil {
		return errors.Wrap(err, "xml select groups")
	}
	for _, row := range rows {
		r := row.(*message.DbGroup)
		groups[r.RecipientId] = *r
	}

	rows, err = SelectStructFromTable(db, message.DbMessage{}, "message")
	if err != nil {
		return errors.Wrap(err, "xml select message")
	}
	for _, row := range rows {
		msg := row.(*message.DbMessage)
		xml := message.NewMessage(*msg)
		message.SetMessageContact(msg, &xml, correspondents, threads, groups)
		msgs.Messages = append(msgs.Messages, xml)
	}

	rows, err = SelectStructFromTable(db, message.DbAttachment{}, "attachment")
	if err != nil {
		return errors.Wrap(err, "xml select attachment")
	}
	for _, row := range rows {
		r := row.(*message.DbAttachment)
		mid := r.MessageId
		msgAttachments[mid] = append(msgAttachments[mid], r)
	}

	for i, msg := range msgs.Messages {
		var messageSize uint64
		id := msg.MessageId
		if attachments, ok := msgAttachments[id]; ok {
			for _, attachment := range attachments {
				xml := message.NewAttachment(*attachment)

				prefix := fmt.Sprintf("%06d", attachment.ID)
				if path, err := findAttachment(pathAttachments, prefix); err != nil {
					if err != os.ErrNotExist {
						return errors.Wrap(err, "find attachment")
					} else if xml.ContentType == "application/x-signal-view-once" {
						log.Printf("attachment %v missing, it was marked 'View Once'", prefix)
					} else if attachment.TransferState > 0 {
						log.Printf("attachment %v missing, transfer state incomplete (%v)", prefix, attachment.TransferState)
					} else {
						log.Printf("attachment file unexpectedly not found under '%v' folder with id = %v", pathAttachments, prefix)
					}
					xml.Src = filepath.Join(pathAttachments, prefix)
				} else if info, err := os.Stat(path); err != nil {
					return errors.Wrap(err, "attachment size")
				} else {
					size := uint64(info.Size())
					if size != attachment.DataSize {
						log.Printf("attachment (id %v) file size (%v) mismatches declared size (%v)", prefix, size, attachment.DataSize)
					}
					messageSize += size
					xml.Src = path
				}
				msg.AttachmentList.Attachments = append(msg.AttachmentList.Attachments, xml)
			}
		}

		sizeString := strconv.FormatUint(messageSize, 10)
		if msg.MSize != "null" && msg.MSize != sizeString {
			log.Printf("MessageID %v declared size %v != calculated size %v\n", id, msg.MSize, sizeString)
		}
		msg.MSize = sizeString

		msgs.Messages[i] = msg
	}

	m := msgs.Messages
	msgs.Count = len(m)
	slices.SortStableFunc(m, func(a, b message.Message) int {
		c := cmp.Compare(a.GroupDate, b.GroupDate)
		if c == 0 {
			c = cmp.Compare(stringPtr(a.GroupName), stringPtr(b.GroupName))
			if c == 0 {
				c = cmp.Compare(a.DateSent, b.DateSent)
			}
		}
		return c
	})

	x, err := xml.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return errors.Wrap(err, "unable to format XML")
	}

	w := types.NewMultiWriter(out)
	w.W([]byte("<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>\n"))
	w.W([]byte("<?xml-stylesheet type=\"text/xsl\" href=\"messages.xsl\" ?>\n"))
	w.W(x)
	return errors.WithMessage(w.Error(), "failed to write out XML")
}

func stringPtr(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}
}

// Synctech() formats the backup into an XML format compatible with
// SMS Backup & Restore by SyncTech. Layout described at their website
// https://www.synctech.com.au/sms-backup-restore/fields-in-xml-backup-files/
func Synctech(db *sql.DB, pathAttachments string, out io.Writer) error {
	recipients := map[int64]message.DbRecipient{}
	smses := &message.SMSes{}
	mmses := []message.MMS{}
	mmsParts := map[int64][]message.MMSPart{} //key: message id

	rows, err := SelectStructFromTable(db, message.DbRecipient{}, "recipient")
	if err != nil {
		return errors.Wrap(err, "xml select recipient")
	}
	for _, row := range rows {
		r := row.(*message.DbRecipient)
		recipients[r.ID] = *r
	}

	rows, err = SelectStructFromTable(db, message.DbSMS{}, "sms")
	if err != nil {
		return errors.Wrap(err, "xml select sms")
	}
	for _, row := range rows {
		sms := row.(*message.DbSMS)
		rcp := recipients[sms.Address]
		xml := message.NewSMS(*sms, rcp)
		smses.SMS = append(smses.SMS, xml)
	}

	rows, err = SelectStructFromTable(db, message.DbMMS{}, "mms")
	if err != nil {
		return errors.Wrap(err, "xml select mms")
	}
	for _, row := range rows {
		mms := row.(*message.DbMMS)
		rcp := recipients[mms.Address]
		xml := message.NewMMS(*mms, rcp)
		mmses = append(mmses, xml)
	}

	rows, err = SelectStructFromTable(db, message.DbPart{}, "part")
	if err != nil {
		return errors.Wrap(err, "xml select part")
	}
	for _, row := range rows {
		r := row.(*message.DbPart)
		mid, xml := message.NewPart(*r)
		mmsParts[mid] = append(mmsParts[mid], xml)
	}

	for _, mms := range mmses {
		var messageSize uint64
		id := mms.MId
		parts, ok := mmsParts[id]
		if ok {
			for i, part := range parts {
				prefix := fmt.Sprintf("%v", part.UniqueId)
				if path, err := findAttachment(pathAttachments, prefix); err != nil {
					if err == os.ErrNotExist {
						log.Printf("No attachment file found with id = %v", id)
					} else {
						return errors.Wrap(err, "find attachment")
					}
				} else if size, data, err := readFileAsBase64(path); err != nil {
					return errors.Wrap(err, "read attachment")
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
			parts = append(parts, message.NewPartText(mms))
			messageSize += uint64(len(*mms.Body))
			if len(parts) == 1 {
				mms.TextOnly = 1
			}
		}
		if len(parts) == 0 {
			continue
		}
		mms.PartList.Parts = parts

		sizeString := strconv.FormatUint(messageSize, 10)
		if mms.MSize != "null" && mms.MSize != sizeString {
			log.Printf("MessageID %v declared size %v != calculated size %v\n", id, mms.MSize, sizeString)
		}
		mms.MSize = sizeString

		if mms.MType == nil {
			if message.SetMMSMessageType(message.MMSSendReq, &mms) != nil {
				panic("logic error: this should never happen")
			}
			smses.MMS = append(smses.MMS, mms)
			if message.SetMMSMessageType(message.MMSRetrieveConf, &mms) != nil {
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

func findAttachment(folder string, prefix string) (string, error) {
	pattern := filepath.Join(folder, prefix + "*")
	if matches, err := filepath.Glob(pattern); err != nil {
		return "", err
	} else if len(matches) == 0 {
		return "", os.ErrNotExist
	} else {
		return matches[0], nil
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
