package setting

// Alipay direct gateway (支付宝直连).
//
// These values are seeded from the options table at startup and written back
// when an administrator saves the payment settings; an empty credential means
// "not configured" and the gateway stays hidden from the wallet.
//
// AlipayPublicKey is Alipay's platform public key (支付宝公钥), used to verify
// asynchronous notifications. It is NOT the application public key: using the
// application key there makes every notification fail verification.
var AlipayAppId = ""

// AlipayPrivateKey is the application private key. Both containers are accepted
// (PKCS#8 "Java 格式" and PKCS#1 "非 Java 格式"), with or without PEM armor; the
// gateway adapter normalizes it.
var AlipayPrivateKey = ""

var AlipayPublicKey = ""

// AlipaySandbox sends requests to openapi-sandbox.dl.alipaydev.com. The sandbox is
// a separate application with its own app id and key pair, so mixing it with
// production credentials fails with isv.invalid-app-id.
var AlipaySandbox = false

var AlipayMinTopUp = 1
