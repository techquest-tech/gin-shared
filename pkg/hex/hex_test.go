package hex

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestHex2(t *testing.T) {

}

func TestHex(t *testing.T) {

	for index := 1; index <= 5000; index++ {
		buffer := bytes.Buffer{}

		buffer.Write([]byte("VTD"))
		//Company 24bit
		company := []byte{0, 0, 0}
		buffer.Write(company)
		managedCode := []byte{0, 0, 0, 0}
		buffer.Write(managedCode)

		searialNo := uint16(index)
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
		fmt.Println(string(dst[:len]))
	}

	// log.Printf("encoding string %s", hex.EncodeToString(src))
}
