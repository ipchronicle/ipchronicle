package admin

import (
	"errors"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpPeriodSeconds int64 = 30

type TOTPEnrollment struct {
	Secret          string
	ProvisioningURI string
}

func generateTOTPEnrollment(username string) (TOTPEnrollment, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "IPChronicle",
		AccountName: username,
		Period:      uint(totpPeriodSeconds),
		SecretSize:  20,
		Secret:      nil,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return TOTPEnrollment{}, err
	}
	return TOTPEnrollment{Secret: key.Secret(), ProvisioningURI: key.URL()}, nil
}

func validateTOTPCode(secret, code string, now time.Time) (int64, error) {
	currentStep := now.UTC().Unix() / totpPeriodSeconds
	for _, offset := range []int64{0, -1, 1} {
		step := currentStep + offset
		valid, err := totp.ValidateCustom(code, secret, time.Unix(step*totpPeriodSeconds, 0), totp.ValidateOpts{
			Period:    uint(totpPeriodSeconds),
			Skew:      0,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if err != nil {
			return 0, err
		}
		if valid {
			return step, nil
		}
	}
	return 0, errors.New("invalid TOTP code")
}
