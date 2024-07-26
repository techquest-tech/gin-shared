package ginshared

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var ToWebsocket = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
