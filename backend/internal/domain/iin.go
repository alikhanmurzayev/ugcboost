package domain

import (
	"errors"
	"fmt"
	"time"
)

// Error codes surfaced by the IIN validator. Handlers or services wrap these
// into a ValidationError (see errors.go) before returning to callers.
const (
	CodeInvalidIIN = "INVALID_IIN"
	CodeUnderAge   = "UNDER_AGE"
)

// Sentinel errors for programmatic checks in callers (service layer wraps
// them into ValidationError with the right code/message).
var (
	ErrIINFormat     = errors.New("iin must be exactly 12 digits")
	ErrIINChecksum   = errors.New("iin checksum is invalid")
	ErrIINCentury    = errors.New("iin century code is invalid")
	ErrIINBirthDate  = errors.New("iin encodes an invalid birth date")
	ErrIINUnderAge = errors.New("applicant is younger than MinCreatorAge")
)

// MinCreatorAge is the minimum age required to submit a creator application.
// Originally 18 per FR3; raised to 21 on 2026-04-25 as the EFW business filter
// (anything stricter — alcohol campaigns etc. — stays downstream).
const MinCreatorAge = 21

// ValidateIIN verifies a Kazakhstani IIN: exactly 12 digits, a valid control
// checksum per the two-pass Republic of Kazakhstan algorithm, a recognisable
// century byte, and a parseable date of birth. On success it returns the
// embedded birth date at UTC midnight. Returned errors are sentinel values so
// callers can map them to the right user-facing code/message.
//
// Custom implementation is used intentionally: no sufficiently maintained
// Go library covers RK-specific checksum and century mapping at the time of
// writing (backend-libraries.md).
func ValidateIIN(iin string) (time.Time, error) {
	var zero time.Time

	if len(iin) != 12 {
		return zero, ErrIINFormat
	}
	var digits [12]int
	for i, r := range iin {
		if r < '0' || r > '9' {
			return zero, ErrIINFormat
		}
		digits[i] = int(r - '0')
	}

	if !iinChecksumValid(digits) {
		return zero, ErrIINChecksum
	}

	year, err := iinYear(digits[0]*10+digits[1], digits[6])
	if err != nil {
		return zero, err
	}
	month := time.Month(digits[2]*10 + digits[3])
	day := digits[4]*10 + digits[5]

	birth := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if birth.Year() != year || birth.Month() != month || birth.Day() != day {
		// time.Date normalises invalid dates (e.g. 02-30 → 03-02). Detect the
		// drift and reject — we don't want to accept bogus dates silently.
		return zero, ErrIINBirthDate
	}

	return birth, nil
}

// AgeYearsOn returns the full years between birth and now.
// Months and days are honoured so a creator turning 18 only tomorrow is not
// counted as 18 today.
func AgeYearsOn(birth, now time.Time) int {
	years := now.Year() - birth.Year()
	anniversary := time.Date(now.Year(), birth.Month(), birth.Day(), 0, 0, 0, 0, birth.Location())
	if now.Before(anniversary) {
		years--
	}
	return years
}

// EnsureAdult returns ErrIINUnderAge if the person born on birth would be
// younger than MinCreatorAge at the given moment.
func EnsureAdult(birth, now time.Time) error {
	if AgeYearsOn(birth, now) < MinCreatorAge {
		return ErrIINUnderAge
	}
	return nil
}

// iinChecksumValid implements the two-pass RK IIN checksum. First pass weights
// are 1..11; if the modulus is 10 the second pass uses 3,4,5,6,7,8,9,10,11,1,2
// and the modulus must still be less than 10. The 12th digit must equal the
// resulting modulus.
func iinChecksumValid(digits [12]int) bool {
	weights1 := [11]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	sum := 0
	for i := 0; i < 11; i++ {
		sum += digits[i] * weights1[i]
	}
	mod := sum % 11
	if mod == 10 {
		weights2 := [11]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 2}
		sum2 := 0
		for i := 0; i < 11; i++ {
			sum2 += digits[i] * weights2[i]
		}
		mod = sum2 % 11
		if mod == 10 {
			return false
		}
	}
	return mod == digits[11]
}

// iinYear maps the two-digit yy and the 7th-digit century byte to a full year.
// Century codes 1/2 → 1800s, 3/4 → 1900s, 5/6 → 2000s, 7/8 → 2100s.
func iinYear(yy, century int) (int, error) {
	switch century {
	case 1, 2:
		return 1800 + yy, nil
	case 3, 4:
		return 1900 + yy, nil
	case 5, 6:
		return 2000 + yy, nil
	case 7, 8:
		return 2100 + yy, nil
	default:
		return 0, fmt.Errorf("%w: century byte %d", ErrIINCentury, century)
	}
}
