package message

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"strconv"
)

// Correspondent represents a 'recipient' DB record.
// New name was chosen to avoid conflict with synctech/Recipient
// and because it also represents "sender".
type Correspondent struct {
	XMLName xml.Name `xml:"correspondent"`
	Number    string   `xml:"number,attr"` // required
}

// Correspondent fields as stored in signal database (relevant subset)
type DbCorrespondent struct {
	ID                int64
	E164              sql.NullString
	GroupId           sql.NullString
	SystemJoinedName  sql.NullString
	ProfileJoinedName sql.NullString
	LastProfileFetch  uint64
}

// NewCorrespondent constructs an XML correspondent struct from a SQL record.
func NewCorrespondent(correspondent DbCorrespondent) (int64, Correspondent) {
	xml := Correspondent{}
	number := StringPtr(correspondent.E164)
	if number == nil {
		xml.Number = "null"
	} else {
		xml.Number = *number
	}

	return correspondent.ID, xml
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
	To           string   `xml:"to,attr"`           // required
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
	FromName    *string  `xml:"from_name,attr"`   // optional
	ToName    *string  `xml:"to_name,attr"`   // optional
}

// https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/database/MessageTable.kt

// Message fields as stored in signal database (relevant subset)
// Fusion of older SMS and MMS tables
type DbMessage struct {
	ID              int64
	FromRecipientId int64
	ToRecipientId   int64  //SMS+MMS Address
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
func NewMessage(msg DbMessage, from DbCorrespondent, to DbCorrespondent) Message {
	xml := Message{
		MessageId:          msg.ID,
		From:           StringRef(from.E164),
		To:           StringRef(to.E164),
		Type:           TranslateSMSType(msg.Type),
		Body:           StringPtr(msg.Body),
		SubscriptionId: msg.SubscriptionId,
		Read:           msg.Read,
		DateSent:     &msg.DateSent,
		DateReceived: msg.DateReceived / 1000,
		CtL:          StringRef(msg.CtL),
		TrId:         StringRef(msg.TrId),
		MType:         IntPtr(msg.MType),
		MSize:        "null",
		ReadableDate: IntToTime(&msg.DateReceived),
		FromName:  StringPtr(from.SystemJoinedName),
		ToName:  StringPtr(to.SystemJoinedName),
	}
	if xml.FromName == nil {
		xml.FromName = StringPtr(from.ProfileJoinedName)
	}
	if xml.ToName == nil {
		xml.ToName = StringPtr(to.ProfileJoinedName)
	}
	if msg.MSize.Valid {
		xml.MSize = strconv.FormatInt(msg.MSize.Int64, 10)
	}
	if v := IntPtr(msg.St); v != nil {
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
	TransferState uint64
	FileName sql.NullString
}

// NewAttachment constructs an XML Attachment struct from a SQL record.
func NewAttachment(attachment DbAttachment) Attachment {
	xml := Attachment{
		ContentType:       StringRef(attachment.ContentType),
		RemoteKey:       StringRef(attachment.RemoteKey),
		RemoteLocation:       StringRef(attachment.RemoteLocation),
		FileName:       StringRef(attachment.FileName),
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

