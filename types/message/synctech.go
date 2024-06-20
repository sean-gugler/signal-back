package message

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"log"
	"strconv"
)

// XML fields are as specified by the page content and .xsd file at:
// https://www.synctech.com.au/sms-backup-restore/fields-in-xml-backup-files/

// Recipient represents a recipient record.
type Recipient struct {
	XMLName xml.Name `xml:"recipient"`
	Phone   string   `xml:"phone,attr"` // required
}

// Recipient fields as stored in signal database (relevant subset)
type DbRecipient struct {
	ID                int64
	Phone             sql.NullString
	GroupId           sql.NullString
	SystemDisplayName sql.NullString
	SignalProfileName sql.NullString
	LastProfileFetch  uint64
}

// NewRecipient constructs an XML recipient struct from a SQL record.
func NewRecipient(recipient DbRecipient) (int64, Recipient) {
	xml := Recipient{}
	phone := StringPtr(recipient.Phone)
	if phone == nil {
		xml.Phone = "null"
	} else {
		xml.Phone = *phone
	}

	return recipient.ID, xml
}

// SMSes holds a set of MMS or SMS records.
type SMSes struct {
	XMLName xml.Name `xml:"smses"`
	Count   int      `xml:"count,attr"`
	MMS     []MMS    `xml:"mms"`
	SMS     []SMS    `xml:"sms"`
}

// SMS represents a Short Message Service record.
type SMS struct {
	XMLName        xml.Name `xml:"sms"`
	Protocol       *uint64  `xml:"protocol,attr"`       // optional
	Address        string   `xml:"address,attr"`        // required
	Date           uint64   `xml:"date,attr"`           // required
	Type           SMSType  `xml:"type,attr"`           // required
	Subject        *string  `xml:"subject,attr"`        // optional
	Body           string   `xml:"body,attr"`           // required
	TOA            *string  `xml:"toa,attr"`            // optional
	SCTOA          *string  `xml:"sc_toa,attr"`         // optional
	ServiceCenter  *string  `xml:"service_center,attr"` // optional
	SubscriptionId int64    `xml:"sub_id,attr"`         // optional
	Read           int64    `xml:"read,attr"`           // required
	Status         int64    `xml:"status,attr"`         // required
	Locked         *uint64  `xml:"locked,attr"`         // optional
	DateSent       *uint64  `xml:"date_sent,attr"`      // optional
	ReadableDate   *string  `xml:"readable_date,attr"`  // optional
	ContactName    *string  `xml:"contact_name,attr"`   // optional
}

// SMS fields as stored in signal database (relevant subset)
type DbSMS struct {
	ID             int64
	Address        int64
	Date           uint64
	DateSent       uint64
	Protocol       sql.NullInt64
	Read           int64
	Status         int64
	Type           int64
	Subject        sql.NullString
	Body           sql.NullString
	ServiceCenter  sql.NullString
	SubscriptionId int64
}

// NewSMS constructs an XML SMS struct from a SQL record.
func NewSMS(sms DbSMS, recipient DbRecipient) SMS {
	xml := SMS{
		Address:        StringRef(recipient.Phone),
		Date:           sms.Date,
		Type:           TranslateSMSType(sms.Type),
		Subject:        StringPtr(sms.Subject),
		Body:           StringRef(sms.Body),
		ServiceCenter:  StringPtr(sms.ServiceCenter),
		SubscriptionId: sms.SubscriptionId,
		Read:           sms.Read,
		Status:         sms.Status,
		DateSent:       &sms.DateSent,
		ReadableDate:   IntToTime(&sms.Date),
		ContactName:    StringPtr(recipient.SystemDisplayName),
	}
	if v := IntPtr(sms.Protocol); v != nil {
		xml.Protocol = v
	}
	if xml.ContactName == nil {
		xml.ContactName = StringPtr(recipient.SignalProfileName)
	}
	return xml
}

type MMSPartList struct {
	XMLName xml.Name `xml:"parts"`
	Parts   []MMSPart
}

// MMS represents a Multimedia Messaging Service record.
type MMS struct {
	XMLName      xml.Name `xml:"mms"`
	PartList     MMSPartList
	Body         *string `xml:"-"`
	TextOnly     uint64  `xml:"text_only,attr"`     // optional
	Sub          string  `xml:"sub,attr"`           // optional (Subject)
	RetrSt       string  `xml:"retr_st,attr"`       // required
	Date         uint64  `xml:"date,attr"`          // required
	CtCls        string  `xml:"ct_cls,attr"`        // required
	SubCs        string  `xml:"sub_cs,attr"`        // required
	Read         uint64  `xml:"read,attr"`          // required
	CtL          string  `xml:"ct_l,attr"`          // required (ContentLocation)
	TrId         string  `xml:"tr_id,attr"`         // required (TransactionID)
	St           string  `xml:"st,attr"`            // required
	MsgBox       uint64  `xml:"msg_box,attr"`       // required
	Address      string  `xml:"address,attr"`       // required (phone number)
	MCls         string  `xml:"m_cls,attr"`         // required
	DTm          string  `xml:"d_tm,attr"`          // required
	ReadStatus   string  `xml:"read_status,attr"`   // required
	CtT          string  `xml:"ct_t,attr"`          // required (ContentType)
	RetrTxtCs    string  `xml:"retr_txt_cs,attr"`   // required
	DRpt         uint64  `xml:"d_rpt,attr"`         // required
	MId          int64   `xml:"m_id,attr"`          // required (Message ID)
	DateSent     uint64  `xml:"date_sent,attr"`     // required
	Seen         uint64  `xml:"seen,attr"`          // required
	MType        *uint64 `xml:"m_type,attr"`        // required (MessageType)
	V            uint64  `xml:"v,attr"`             // required
	Exp          string  `xml:"exp,attr"`           // required
	Pri          uint64  `xml:"pri,attr"`           // required
	Rr           uint64  `xml:"rr,attr"`            // required (Read Report)
	RespTxt      string  `xml:"resp_txt,attr"`      // required
	RptA         string  `xml:"rpt_a,attr"`         // required
	Locked       uint64  `xml:"locked,attr"`        // required
	RetrTxt      string  `xml:"retr_txt,attr"`      // required
	RespSt       string  `xml:"resp_st,attr"`       // required
	MSize        string  `xml:"m_size,attr"`        // required (MessageSize)
	SimSlot      *string `xml:"sim_slot,attr"`      // optional
	ReadableDate *string `xml:"readable_date,attr"` // optional
	ContactName  *string `xml:"contact_name,attr"`  // optional
}

// MMS fields as stored in signal database (relevant subset)
type DbMMS struct {
	ID           int64
	Address      int64
	Read         uint64
	MType        uint64         //MessageType
	MSize        sql.NullInt64  //MessageSize
	CtL          sql.NullString //ContentLocation
	Date         uint64
	DateReceived uint64
	Body         sql.NullString
	TrId         sql.NullString //TransactionID
}

// NewMMS constructs an XML MMS struct from a SQL record.
func NewMMS(mms DbMMS, recipient DbRecipient) MMS {
	xml := MMS{
		TextOnly:     0,
		Sub:          "null",
		RetrSt:       "null",
		Date:         mms.DateReceived,
		CtCls:        "null",
		SubCs:        "null",
		Body:         StringPtr(mms.Body),
		Read:         mms.Read,
		CtL:          StringRef(mms.CtL),
		TrId:         StringRef(mms.TrId),
		St:           "null",
		MCls:         "personal",
		DTm:          "null",
		ReadStatus:   "null",
		CtT:          "application/vnd.wap.multipart.related",
		RetrTxtCs:    "null",
		DateSent:     mms.Date / 1000,
		Seen:         mms.Read,
		Exp:          "null",
		RespTxt:      "null",
		RptA:         "null",
		Locked:       0,
		RetrTxt:      "null",
		RespSt:       "null",
		MSize:        "null",
		ReadableDate: IntToTime(&mms.DateReceived),
		Address:      StringRef(recipient.Phone),
		ContactName:  StringPtr(recipient.SystemDisplayName),
		MId:          mms.ID,
	}
	if xml.ContactName == nil {
		xml.ContactName = StringPtr(recipient.SignalProfileName)
	}
	if mms.MSize.Valid {
		xml.MSize = strconv.FormatInt(mms.MSize.Int64, 10)
	}
	if err := SetMMSMessageType(mms.MType, &xml); err != nil {
		body := StringPtr(mms.Body)
		if body == nil {
			s := "null"
			body = &s
		}
		log.Fatalf("%v\nplease report this issue, as well as (if possible) details about the MMS\nID = %d, body = %s\n\n%v", err, mms.ID, *body, mms)
	}

	return xml
}

// MMSPart holds a data blob for an MMS.
type MMSPart struct {
	XMLName  xml.Name `xml:"part"`
	DataSize uint64   `xml:"-"`
	UniqueId uint64   `xml:"-"`
	Seq      uint64   `xml:"seq,attr"`   // required
	Ct       string   `xml:"ct,attr"`    // required (ContentType)
	Name     string   `xml:"name,attr"`  // required
	ChSet    string   `xml:"chset,attr"` // required (CharacterSet)
	Cd       string   `xml:"cd,attr"`    // required (ContentDisposition)
	Fn       string   `xml:"fn,attr"`    // required
	CID      string   `xml:"cid,attr"`   // required
	Cl       string   `xml:"cl,attr"`    // required (ContentLocation)
	CttS     string   `xml:"ctt_s,attr"` // required
	CttT     string   `xml:"ctt_t,attr"` // required
	Text     string   `xml:"text,attr"`  // required
	Data     *string  `xml:"data,attr"`  // optional
}

// Part fields as stored in signal database (relevant subset)
type DbPart struct {
	Mid      int64  //MessageId
	Seq      int64  //Sequence
	Ct       string //ContentType
	Name     sql.NullString
	Chset    sql.NullString //CharacterSet
	Cd       sql.NullString //ContentDisposition
	Fn       sql.NullString
	Cid      sql.NullString
	Cl       sql.NullString //ContentLocation
	CttS     sql.NullString //NullInt64
	CttT     sql.NullString
	DataSize uint64
	UniqueId uint64
}

// NewPart constructs an XML Part struct from a SQL record.
func NewPart(part DbPart) (int64, MMSPart) {
	xml := MMSPart{
		DataSize: part.DataSize,
		UniqueId: part.UniqueId,
		Seq:      uint64(part.Seq),
		Ct:       part.Ct,
		Name:     StringRef(part.Name),
		ChSet:    StringRef(part.Chset),
		Cd:       StringRef(part.Cd),
		Fn:       StringRef(part.Fn),
		CID:      StringRef(part.Cid),
		Cl:       StringRef(part.Cl),
		CttS:     StringRef(part.CttS),
		CttT:     StringRef(part.CttT),
	}
	if xml.ChSet == "" {
		xml.ChSet = CharsetUTF8
	}

	return part.Mid, xml
}

// NewPartText constructs an XML Part struct from an MMS body.
func NewPartText(mms MMS) MMSPart {
	null := "null"
	chset := CharsetUTF8
	cl := fmt.Sprintf("txt%06d.txt", mms.MId)

	xml := MMSPart{
		Seq:   0,
		Ct:    "text/plain",
		Name:  null,
		ChSet: chset,
		Cd:    null,
		Fn:    null,
		CID:   null,
		Cl:    cl,
		CttS:  null,
		CttT:  null,
		Text:  *mms.Body,
	}

	return xml
}

