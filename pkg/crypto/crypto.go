package crypto

import (
	"crypto/rand"
	"encoding/base64"
)

var remoteParamKey = []byte{
	0x74, 0xB2, 0x92, 0x5B, 0x4A, 0x21, 0xDA,
}

func ObfuscateRemoteParam(raw []byte) {
	for i := 0; i < len(raw); i++ {
		raw[i] ^= remoteParamKey[i%7]
	}
}

var authKey = []byte{
	0xB6, 0xD5, 0x92, 0xC4, 0xA7, 0x83, 0xE1,
}

func ObfuscateAuthParam(raw []byte) {
	for i := 0; i < len(raw); i++ {
		raw[i] ^= authKey[i%7]
	}
}

// Generate random password for the connection.
// In the original implementation this uses all ASCII characters no just base64 alphabet.
func MakePassword() string {
	token := new([6]byte)
	_, err := rand.Read(token[:])
	if err != nil {
		return ""
	}
	return base64.StdEncoding.WithPadding(base64.NoPadding).EncodeToString(token[:])
}
