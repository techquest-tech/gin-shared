package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/techquest-tech/gin-shared/pkg/core"
)

func TestEncrypt(t *testing.T) {
	raw := "hello, AES, it's security now"
	core.Provide(func() core.ConfigSecret {
		return core.ConfigSecret("mac9jz5ul91s6of46nuco1tnq75ki037")
	})

	err := core.GetContainer().Invoke(func(secret core.ConfigSecret) error {
		out, err := core.Encrypt(secret, []byte(raw))
		if err != nil {
			return err
		}
		src, err := core.Decrypt(secret, out)
		if err != nil {
			return err
		}
		assert.Equal(t, raw, string(src))
		return nil
	})
	assert.Nil(t, err)
}
