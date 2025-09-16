package main

import (
	"github.com/pquerna/otp/totp"
)

func GenMfa(user string) (secret string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "openvpn-web",
		AccountName: user,
	})
	if err != nil {
		return "", err
	}

	return key.Secret(), nil
}

func ValidateMfa(passcode, key string) bool {
	return totp.Validate(passcode, key)
}
