package core

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

var commonIV = []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}

func Encrypt(key, raw []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	fmt.Printf("block size: %d", block.BlockSize())

	cfb := cipher.NewCFBEncrypter(block, commonIV)

	out := make([]byte, len(raw))
	cfb.XORKeyStream(out, raw)
	return out, nil
}

func Decrypt(key, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cfb := cipher.NewCFBDecrypter(block, commonIV)

	out := make([]byte, len(src))
	cfb.XORKeyStream(out, src)
	return out, nil
}
