package types

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
	// "log"
	// "encoding/hex"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/xeals/signal-back/signal"
	"golang.org/x/crypto/hkdf"
)

// ATTACHMENT_BUFFER_SIZE is the size of the buffer in bytes used for decrypting attachments.
// Larger values of this consume more memory, but have been measured to not actually decrease
// the overall time taken.
const ATTACHMENT_BUFFER_SIZE = 8192

// ProtoCommitHash is the commit hash of the Signal Protobuf spec.
var ProtoCommitHash = "d6610f0"

// BackupFile stores information about a given backup file.
//
// Decrypting a backup is done by consuming the underlying file buffer. Attemtping to read from a
// BackupFile after it is consumed will return an error.
//
// Closing the underlying file handle is the responsibilty of the programmer if implementing the
// iteration manually, or is done as part of the Consume method.
type BackupFile struct {
	file      *os.File
	FileSize  int64
	CipherKey []byte
	MacKey    []byte
	Mac       hash.Hash
	IV        []byte
	Salt      []byte
	Counter   uint32
}

// NewBackupFile initialises a backup file for reading using the provided path
// and password.
func NewBackupFile(path, password string) (*BackupFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get size of backup file")
	}
	size := info.Size()

	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "unable to open backup file")
	}

	headerLengthBytes := make([]byte, 4)
	_, err = io.ReadFull(file, headerLengthBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read headerLengthBytes")
	}
	headerLength := bytesToUint32(headerLengthBytes)

	headerFrame := make([]byte, headerLength)
	_, err = io.ReadFull(file, headerFrame)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read headerFrame")
	}
	frame := &signal.BackupFrame{}
	if err = proto.Unmarshal(headerFrame, frame); err != nil {
		return nil, errors.Wrap(err, "failed to decode header")
	}

	version := frame.Header.GetVersion()
	if version > 0 {
		return nil, errors.New(fmt.Sprintf("File Version %d not yet supported", version))
	}

	iv := frame.Header.Iv
	if len(iv) != 16 {
		return nil, errors.New("No IV in header")
	}

	key := backupKey(password, frame.Header.Salt)
	derived := deriveSecrets(key, []byte("Backup Export"))
	cipherKey := derived[:32]
	macKey := derived[32:]

	return &BackupFile{
		file:      file,
		FileSize:  size,
		CipherKey: cipherKey,
		MacKey:    macKey,
		Mac:       hmac.New(crypto.SHA256.New, macKey),
		IV:        iv,
		Salt:      frame.Header.Salt,
		Counter:   bytesToUint32(iv),
	}, nil
}

// Frame returns the next frame in the file.
func (bf *BackupFile) Frame() (uint32, *signal.BackupFrame, error) {
	length := make([]byte, 4)
	_, err := io.ReadFull(bf.file, length)
	if err != nil {
		return 0, nil, err
	}

	frameLength := bytesToUint32(length)
	frame := make([]byte, frameLength)

	io.ReadFull(bf.file, frame)

	messageLength := len(frame) - 10
	theirMac := frame[messageLength:]

	bf.Mac.Reset()
	bf.Mac.Write(frame[:messageLength])
	ourMac := bf.Mac.Sum(nil)[:10]

	if !hmac.Equal(theirMac, ourMac) {
		// log.Printf("MAC expect %s found %s", hex.EncodeToString(ourMac), hex.EncodeToString(theirMac))
		return 0, nil, errors.New("Decryption error, wrong password")
	}

	uint32ToBytes(bf.IV, bf.Counter)
	bf.Counter++

	aesCipher, err := aes.NewCipher(bf.CipherKey)
	if err != nil {
		return 0, nil, errors.New("Bad cipher")
	}
	stream := cipher.NewCTR(aesCipher, bf.IV)

	output := make([]byte, messageLength)
	stream.XORKeyStream(output, frame[:messageLength])

	decoded := new(signal.BackupFrame)
	proto.Unmarshal(output, decoded)

	return frameLength, decoded, nil
}

// DecryptAttachment reads the attachment immediately next in the file's bytes, using a streaming
// intermediate buffer of size ATTACHMENT_BUFFER_SIZE.
func (bf *BackupFile) DecryptAttachment(length uint32, out io.Writer) error {
	// if length == 0 {
	// 	return errors.New("can't read attachment of length 0")
	// }

	uint32ToBytes(bf.IV, bf.Counter)
	bf.Counter++

	if out == nil {
		_, err := bf.file.Seek(int64(length + 10), io.SeekCurrent)
		if err != nil {
			return errors.Wrap(err, "failed to seek over attachment data")
		}
		return nil
	}

	aesCipher, err := aes.NewCipher(bf.CipherKey)
	if err != nil {
		return errors.New("Bad cipher")
	}
	stream := cipher.NewCTR(aesCipher, bf.IV)
	bf.Mac.Reset()
	bf.Mac.Write(bf.IV)

	buf := make([]byte, ATTACHMENT_BUFFER_SIZE)
	output := make([]byte, len(buf))

	for length > 0 {
		// Go can't read an arbitrary number of bytes,
		// so we have to downsize the buffers instead.
		if length < ATTACHMENT_BUFFER_SIZE {
			buf = make([]byte, length)
			output = make([]byte, length)
		}
		n, err := bf.file.Read(buf)
		if err != nil {
			return errors.Wrap(err, "failed to read attachment data")
		}
		bf.Mac.Write(buf)

		stream.XORKeyStream(output, buf)
		if _, err = out.Write(output); err != nil {
			return errors.Wrap(err, "can't write to output")
		}

		length -= uint32(n)
	}

	theirMac := make([]byte, 10)
	io.ReadFull(bf.file, theirMac)
	ourMac := bf.Mac.Sum(nil)[:10]

	if !hmac.Equal(theirMac, ourMac) {
		// log.Printf("MAC expect %s found %s", hex.EncodeToString(ourMac), hex.EncodeToString(theirMac))
		return errors.New("Decryption error, wrong password")
	}

	return nil
}

// ConsumeFuncs stores parameters for a Consume operation.
type ConsumeFuncs struct {
	FrameFunc      func(*signal.BackupFrame, int64, uint32) error
	AttachmentFunc func(*signal.Attachment) error
	AvatarFunc     func(*signal.Avatar) error
	StickerFunc    func(*signal.Sticker) error
	PreferenceFunc func(*signal.SharedPreference) error
	KeyValueFunc   func(*signal.KeyValue) error
	StatementFunc  func(*signal.SqlStatement) error
}

// Consume iterates over the backup file using the fields in the provided ConsumeFuncs. When a
// BackupFrame is encountered, the matching function will run.
//
// If any image-related functions are nil (e.g., AttachmentFunc) the default will be to discard the
// next *n* bytes, where n is the Attachment.Length.
//
// The underlying file is closed at the end of the method, and the backup file should be considered
// spent.
func (bf *BackupFile) Consume(fns ConsumeFuncs) error {
	var (
		pos     int64
		length  uint32
		f       *signal.BackupFrame
		err     error
	)

	defer bf.Close()

	// frame attachments MUST be handled, even if discarded
	if fns.AttachmentFunc == nil {
		fns.AttachmentFunc = func(a *signal.Attachment) error {
			return bf.DecryptAttachment(a.GetLength(), nil)
		}
	}
	if fns.AvatarFunc == nil {
		fns.AvatarFunc = func(a *signal.Avatar) error {
			return bf.DecryptAttachment(a.GetLength(), nil)
		}
	}
	if fns.StickerFunc == nil {
		fns.StickerFunc = func(a *signal.Sticker) error {
			return bf.DecryptAttachment(a.GetLength(), nil)
		}
	}

	for {
		pos, err = bf.file.Seek(0, io.SeekCurrent)
		if err != nil {
			return errors.Wrap(err, "consume [seek]")
		}

		length, f, err = bf.Frame()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if fn := fns.FrameFunc; fn != nil {
			if err = fn(f, pos, length); err != nil {
				return errors.Wrap(err, "consume [frame]")
			}
		}

		if fn := fns.AttachmentFunc; fn != nil {
			if data := f.GetAttachment(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [attachment]")
				}
			}
		}
		if fn := fns.AvatarFunc; fn != nil {
			if data := f.GetAvatar(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [avatar]")
				}
			}
		}
		if fn := fns.StickerFunc; fn != nil {
			if data := f.GetSticker(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [sticker]")
				}
			}
		}
		if fn := fns.PreferenceFunc; fn != nil {
			if data := f.GetPreference(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [preference]")
				}
			}
		}
		if fn := fns.KeyValueFunc; fn != nil {
			if data := f.GetKeyValue(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [keyvalue]")
				}
			}
		}
		if fn := fns.StatementFunc; fn != nil {
			if data := f.GetStatement(); data != nil {
				if err = fn(data); err != nil {
					return errors.Wrap(err, "consume [statement]")
				}
			}
		}
	}

	return nil
}

func (bf *BackupFile) Close() error {
	return bf.file.Close()
}

func backupKey(password string, salt []byte) []byte {
	digest := crypto.SHA512.New()
	input := []byte(strings.Replace(strings.TrimSpace(password), " ", "", -1))
	hash := input

	if salt != nil {
		digest.Write(salt)
	}

	for i := 0; i < 250000; i++ {
		digest.Write(hash)
		digest.Write(input)
		hash = digest.Sum(nil)
		digest.Reset()
	}

	return hash[:32]
}

func deriveSecrets(input, info []byte) []byte {
	sha := crypto.SHA256.New
	salt := make([]byte, sha().Size())
	okm := make([]byte, 64)

	hkdf := hkdf.New(sha, input, salt, info)
	_, err := io.ReadFull(hkdf, okm)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to generate hashes:", err.Error())
	}

	return okm
}

func bytesToUint32(b []byte) (val uint32) {
	val |= uint32(b[3])
	val |= uint32(b[2]) << 8
	val |= uint32(b[1]) << 16
	val |= uint32(b[0]) << 24
	return
}

func uint32ToBytes(b []byte, val uint32) {
	b[3] = byte(val)
	b[2] = byte(val >> 8)
	b[1] = byte(val >> 16)
	b[0] = byte(val >> 24)
	return
}
