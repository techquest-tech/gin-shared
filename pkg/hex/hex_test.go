package hex

import (
	"bytes"
	"encoding/hex"
	"log"
	"testing"
)

func TestHex2(t *testing.T) {

}

func TestHex(t *testing.T) {
	buffer := bytes.Buffer{}

	buffer.Write([]byte("VTD"))
	//Company 24bit
	company := []byte{0, 0, 0}
	buffer.Write(company)
	managedCode := []byte{0, 0, 0, 0}
	buffer.Write(managedCode)

	searialNo := uint16(5000)
	h, l := uint8(searialNo>>8), uint8(searialNo&0xff)
	buffer.WriteByte(h)
	buffer.WriteByte(l)

	src := buffer.Bytes()
	maxlen := hex.EncodedLen(len(src))

	dst := make([]byte, maxlen)
	len := hex.Encode(dst, src)
	// assert.Nil(t, err)

	// log.Printf("decode lens %d", len)

	// result := hex.Dump(dst)
	log.Printf("result: %s", string(dst[:len]))
	log.Printf("encoding string %s", hex.EncodeToString(src))
}
