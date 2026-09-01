package setting

// WeChat Pay Native (API v3) configuration. Sensitive values are kept in the
// option store but are never returned by the public option endpoint.
var (
	WechatEnabled                 bool
	WechatMchID                   string
	WechatAppID                   string
	WechatApiV3Key                string
	WechatCertificateSerialNumber string
	WechatPrivateKey              string
	WechatMerchantCertificate     string
	WechatPayPublicKeyID          string
	WechatPayPublicKey            string
	WechatPlatformCertificate     string
	WechatNotifyURL               string
	WechatOrderExpireMinutes      int = 120
)
