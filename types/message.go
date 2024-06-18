package types

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// SMSType is an SMS type as defined by the XML backup spec.
type SMSType int64

// SMS types
const (
	SMSInvalid  SMSType = iota // 0
	SMSReceived                // 1
	SMSSent                    // 2
	SMSDraft                   // 3
	SMSOutbox                  // 4
	SMSFailed                  // 5
	SMSQueued                  // 6
)

// MMS message types as defined by the MMS Encapsulation Protocol.
// See: http://www.openmobilealliance.org/release/MMS/V1_2-20050429-A/OMA-MMS-ENC-V1_2-20050301-A.pdf
const (
	MMSSendReq           uint64 = iota + 128 // 128
	MMSSendConf                              // 129
	MMSNotificationInd                       // 130
	MMSNotifyResponseInd                     // 131
	MMSRetrieveConf                          // 132
	MMSAckknowledgeInd                       // 133
	MMSDeliveryInd                           // 134
	MMSReadRecInd                            // 135
	MMSReadOrigInd                           // 136
	MMSForwardReq                            // 137
	MMSForwardConf                           // 138
	MMSMBoxStoreReq                          // 139
	MMSMBoxStoreConf                         // 140
	MMSMBoxViewReq                           // 141
	MMSMBoxViewConf                          // 142
	MMSMBoxUploadReq                         // 143
	MMSMBoxUploadConf                        // 144
	MMSMBoxDeleteReq                         // 145
	MMSMBoxDeleteConf                        // 146
	MMSMBoxDescr                             // 147
)

// XML fields are as specified by the page content and .xsd file at:
// https://www.synctech.com.au/sms-backup-restore/fields-in-xml-backup-files/

// Recipient represents a recipient record.
type Recipient struct {
	XMLName xml.Name `xml:"recipient"`
	From    string   `xml:"from,attr"` // required
}

// Recipient fields as stored in signal database (relevant subset)
type DbRecipient struct {
	ID                int64
	E164              sql.NullString
	GroupId           sql.NullString
	SystemJoinedName  sql.NullString
	ProfileJoinedName sql.NullString
	LastProfileFetch  uint64
}

// NewRecipient constructs an XML recipient struct from a SQL record.
func NewRecipient(recipient DbRecipient) (int64, Recipient) {
	xml := Recipient{}
	from := stringPtr(recipient.E164)
	if from == nil {
		xml.From = "null"
	} else {
		xml.From = *from
	}

	return recipient.ID, xml
}

// Messages holds a set of Message records.
type Messages struct {
	XMLName  xml.Name  `xml:"messages"`
	Count    int       `xml:"count,attr"`
	Messages []Message `xml:"message"`
}

type AttachmentList struct {
	XMLName xml.Name `xml:"attachments"`
	Attachments   []Attachment
}

type Message struct {
	XMLName      xml.Name `xml:"message"`
	AttachmentList     AttachmentList
	From           string   `xml:"from,attr"`           // required
	DateSent       *uint64  `xml:"date_sent,attr"`      // optional
	DateReceived           uint64   `xml:"date_received,attr"`           // required
	Type           SMSType  `xml:"type,attr"`           // required
	Body           *string   `xml:"body,attr"`           // required
	SubscriptionId int64    `xml:"sub_id,attr"`         // optional
	Read           int64    `xml:"read,attr"`           // required
	Status         *uint64    `xml:"status,attr"`         // required
	CtL          string  `xml:"ct_l,attr"`          // required (ContentLocation)
	TrId         string  `xml:"tr_id,attr"`         // required (TransactionID)
	MessageId          int64   `xml:"message_id,attr"`          // required
	MType        *uint64 `xml:"m_type,attr"`        // required (MessageType)
	MSize        string  `xml:"m_size,attr"`        // required (MessageSize)
	ReadableDate   *string  `xml:"readable_date,attr"`  // optional
	ContactName    *string  `xml:"contact_name,attr"`   // optional
}

// https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/database/MessageTable.kt

// Message fields as stored in signal database (relevant subset)
// Fusion of older SMS and MMS tables
type DbMessage struct {
	ID              int64
	FromRecipientId int64  //SMS+MMS Address
	DateReceived    uint64 //SMS Date, MMS DateReceived
	DateSent        uint64 //SMS DateSent, MMS Date
	Read            int64
	St              sql.NullInt64 //SMS Status
	Type            int64 //SMS Type, MMS MsgBox
	Body            sql.NullString
	SubscriptionId  int64
	MType           sql.NullInt64  //MessageType
	MSize           sql.NullInt64  //MessageSize
	CtL             sql.NullString //ContentLocation
	TrId            sql.NullString //TransactionID
}

// NewMessage constructs an XML Message struct from a SQL record.
func NewMessage(msg DbMessage, recipient DbRecipient) Message {
	xml := Message{
		MessageId:          msg.ID,
		From:           stringRef(recipient.E164),
		Type:           translateSMSType(msg.Type),
		Body:           stringPtr(msg.Body),
		SubscriptionId: msg.SubscriptionId,
		Read:           msg.Read,
		DateSent:     &msg.DateSent,
		DateReceived: msg.DateReceived / 1000,
		CtL:          stringRef(msg.CtL),
		TrId:         stringRef(msg.TrId),
		MType:         intPtr(msg.MType),
		MSize:        "null",
		ReadableDate: intToTime(&msg.DateReceived),
		ContactName:  stringPtr(recipient.SystemJoinedName),
	}
	if xml.ContactName == nil {
		xml.ContactName = stringPtr(recipient.ProfileJoinedName)
	}
	if msg.MSize.Valid {
		xml.MSize = strconv.FormatInt(msg.MSize.Int64, 10)
	}
	if v := intPtr(msg.St); v != nil {
		xml.Status = v
	}
	return xml
}

// Attachment holds a single attachment for a Message.
type Attachment struct {
	XMLName  xml.Name `xml:"attachment"`
	DataSize uint64   `xml:"-"`
	ContentType string   `xml:"content_type,attr"`
	RemoteKey     string   `xml:"remote_key,attr"` // required
	RemoteLocation     string   `xml:"remote_location,attr"` // required
	FileName     string   `xml:"file_name,attr"` // required
	Src     string   `xml:"src,attr"`
	Text     string   `xml:"text,attr"`  // required
	Data     *string  `xml:"data,attr"`  // optional
}

// Attachment fields as stored in signal database (relevant subset)
type DbAttachment struct {
	ID              int64
	MessageId      int64
	DataSize uint64
	ContentType       sql.NullString
	RemoteKey       sql.NullString
	RemoteLocation       sql.NullString
	FileName sql.NullString
}

// NewAttachment constructs an XML Attachment struct from a SQL record.
func NewAttachment(attachment DbAttachment) Attachment {
	xml := Attachment{
		ContentType:       stringRef(attachment.ContentType),
		RemoteKey:       stringRef(attachment.RemoteKey),
		RemoteLocation:       stringRef(attachment.RemoteLocation),
		FileName:       stringRef(attachment.FileName),
		DataSize: attachment.DataSize,
	}

	return xml
}

// NewAttachmentText constructs an XML Attachment struct from an MMS body.
func NewAttachmentText(msg Message) Attachment {
	null := "null"
	remoteLocation := fmt.Sprintf("txt%06d.txt", msg.MessageId)

	xml := Attachment{
		ContentType:    "text/plain",
		RemoteKey:  null,
		RemoteLocation:  remoteLocation,
		FileName:  null,
		Text:  *msg.Body,
	}

	return xml
}

func SetMMSMessageType(messageType uint64, msg *Message) error {
	switch messageType {
	case MMSSendReq:
		// mms.MsgBox = 2
		// mms.V = 18
		break
	case MMSNotificationInd:
		// We can safely ignore this case.
		break
	case MMSRetrieveConf:
		// mms.MsgBox = 1
		// mms.V = 16
		break
	default:
		return errors.Errorf("unsupported message type %v encountered", messageType)
	}

	msg.MType = &messageType
	return nil
}

func translateSMSType(t int64) SMSType {
	// Just get the lowest 5 bits, because everything else is masking.
	// https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/database/MessageTypes.java
	v := uint8(t) & 0x1F

	if 1 <= v && v <= 18 {
		return SMSInvalid
	}

	switch v {
	case 20: // signal inbox
		return SMSReceived
	case 21: // signal outbox
		return SMSOutbox
	case 22: // signal sending
		return SMSQueued
	case 23: // signal sent
		return SMSSent
	case 24: // signal failed
		return SMSFailed
	case 25: // pending secure SMS fallback
		return SMSQueued
	case 26: // pending insecure SMS fallback
		return SMSQueued
	case 27: // signal draft
		return SMSDraft

	default:
		log.Fatalf("undefined SMS type: %#v\nplease report this issue, as well as (if possible) details about the SMS,\nsuch as whether it was sent, received, drafted, etc.\n", t)
		log.Fatalf("note that the output XML may not properly import to Signal\n")
		return SMSInvalid
	}
}

func intToTime(n *uint64) *string {
	if n == nil {
		return nil
	}
	unix := time.Unix(int64(*n)/1000, 0)
	t := unix.Format("Jan 02, 2006 3:04:05 PM")
	return &t
}

func stringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func stringRef(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "null"
}

func intPtr(ns sql.NullInt64) *uint64 {
	if ns.Valid {
		u := uint64(ns.Int64)
		return &u
	}
	return nil
}
