package message

import (
	"database/sql"
	"log"
	"time"

	"github.com/pkg/errors"
)

// Character sets as specified by IANA.
const (
	CharsetASCII = "3"
	CharsetUTF8  = "106"
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

func SetMMSMessageType(messageType uint64, mms *MMS) error {
	switch messageType {
	case MMSSendReq:
		mms.MsgBox = 2
		mms.V = 18
		break
	case MMSNotificationInd:
		// We can safely ignore this case.
		break
	case MMSRetrieveConf:
		mms.MsgBox = 1
		mms.V = 16
		break
	default:
		return errors.Errorf("unsupported message type %v encountered", messageType)
	}

	mms.MType = &messageType
	return nil
}

func TranslateSMSType(t int64) SMSType {
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

func IntToTime(n *uint64) *string {
	if n == nil {
		return nil
	}
	unix := time.Unix(int64(*n)/1000, 0)
	t := unix.Format("Jan 02, 2006 3:04:05 PM")
	return &t
}

func StringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func StringRef(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return "null"
}

func IntPtr(ns sql.NullInt64) *uint64 {
	if ns.Valid {
		u := uint64(ns.Int64)
		return &u
	}
	return nil
}

func IntRef(ns sql.NullInt64) uint64 {
	if ns.Valid {
		u := uint64(ns.Int64)
		return u
	}
	return 0
}
