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

type DbGroup struct {
	GroupId     string
	RecipientId int64
	Title       sql.NullString
	Timestamp   sql.NullInt64
}

type DbThread struct {
	ID          int64
	RecipientId int64
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
	DateSent       uint64  `xml:"date_sent,attr"`      // optional
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
	ContactName           *string   `xml:"contact_name,attr"`           // required
	GroupName           *string   `xml:"group_name,attr"`           // required
	GroupDate       uint64  `xml:"-"`      // optional
}

// https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/database/MessageTable.kt

// Message fields as stored in signal database (relevant subset)
// Fusion of older SMS and MMS tables
type DbMessage struct {
	ID              int64
	ThreadId        int64
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
func NewMessage(msg DbMessage) Message {
	xml := Message{
		MessageId:          msg.ID,
		Type:           TranslateSMSType(msg.Type),
		Body:           StringPtr(msg.Body),
		SubscriptionId: msg.SubscriptionId,
		DateSent:     msg.DateSent,
		DateReceived: msg.DateReceived,
		Read:           msg.Read,
		Status:       IntPtr(msg.St),
		CtL:          StringRef(msg.CtL),
		TrId:         StringRef(msg.TrId),
		MType:         IntPtr(msg.MType),
		MSize:        "null",
		ReadableDate: IntToTime(&msg.DateSent),
	}
	if v := IntPtr(msg.MSize); v != nil {
		xml.MSize = strconv.FormatUint(*v, 10)
	}
	return xml
}

func SetMessageContact(msg *DbMessage, xml *Message, correspondents map[int64]DbCorrespondent, threads map[int64]DbThread, groups map[int64]DbGroup) {
	if thread, ok := threads[msg.ThreadId]; ok {
		tid := thread.RecipientId
		
		if group, ok := groups[tid]; ok {
			name := StringPtr(group.Title)
			if name == nil || *name == "" {
				generic := fmt.Sprintf("Group%d", tid)
				name = &generic
			}
			xml.GroupName = name
			xml.GroupDate = IntRef(group.Timestamp)
			xml.Type = SMSReceived
		}
	}

	id := msg.ToRecipientId
	if xml.Type == SMSReceived {
		id = msg.FromRecipientId
	}

	if correspondent, ok := correspondents[id]; ok {
		name := StringPtr(correspondent.SystemJoinedName)
		if name == nil {
			name = StringPtr(correspondent.ProfileJoinedName)
		}
		if name == nil {
			name = StringPtr(correspondent.E164)
		}
		xml.ContactName = name
	}
}

// Attachment holds a single attachment for a Message.
type Attachment struct {
	XMLName  xml.Name `xml:"attachment"`
	DataSize uint64   `xml:"-"`
	ContentType string   `xml:"content_type,attr"`
	RemoteKey     string   `xml:"remote_key,attr"` // required
	RemoteLocation     string   `xml:"remote_location,attr"` // required
	FileName     string   `xml:"file_name,attr"` // required
	Src     *string   `xml:"src,attr"`
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
