package lib

import "net/mail"

func IsEmail(str string) bool {
	address, err := mail.ParseAddress(str)
	if err != nil {
		return false
	}
	return str == address.Address
}
