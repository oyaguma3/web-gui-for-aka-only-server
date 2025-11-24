package validators

import (
	"regexp"
)

var (
	imsiRegex  = regexp.MustCompile(`^[0-9]{15}$`)
	hex32Regex = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	hex12Regex = regexp.MustCompile(`^[0-9a-fA-F]{12}$`)
	hex4Regex  = regexp.MustCompile(`^[0-9a-fA-F]{4}$`)
)

func ValidateIMSI(imsi string) bool {
	return imsiRegex.MatchString(imsi)
}

func ValidateKey(key string) bool {
	return hex32Regex.MatchString(key)
}

func ValidateOPC(opc string) bool {
	return hex32Regex.MatchString(opc)
}

func ValidateSQN(sqn string) bool {
	return hex12Regex.MatchString(sqn)
}

func ValidateAMF(amf string) bool {
	return hex4Regex.MatchString(amf)
}
