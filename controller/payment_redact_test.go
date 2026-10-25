package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactKeyMaterial(t *testing.T) {
	key := strings.Repeat("FAKEKEYMATERIAL", 20)
	in := "initialize Alipay client: 私钥解析出错 [-----BEGIN RSA PRIVATE KEY-----\n" + key + "\n-----END RSA PRIVATE KEY-----\n]"

	out := redactKeyMaterial(in)
	assert.NotContains(t, out, "BEGIN")
	assert.NotContains(t, out, "FAKEKEY")
	assert.Contains(t, out, "私钥解析出错")
	assert.LessOrEqual(t, len([]rune(out)), 260)
}

func TestRedactKeyMaterialLeavesOrdinaryErrorsAlone(t *testing.T) {
	in := `{"code":"40002","msg":"Invalid Arguments","sub_code":"isv.invalid-app-id"}`
	assert.Equal(t, in, redactKeyMaterial(in))
}
