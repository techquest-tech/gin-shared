package auth

import (
	"bytes"

	"github.com/techquest-tech/gin-shared/pkg/ginshared"
)

// type SignedRequest struct {

// }

func SignRequest(app, ts, secret string, request []byte) (string, error) {
	buf := bytes.Buffer{}
	buf.WriteString("app=")
	buf.WriteString(app)
	buf.WriteString("&secret=")
	buf.WriteString(secret)
	buf.WriteString("&timestamp=")
	buf.WriteString(ts)
	buf.WriteString("&body=")
	buf.Write(request)

	signed := ginshared.MD5(buf.Bytes())

	return signed, nil
}
