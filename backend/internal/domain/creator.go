package domain

import "errors"

// ErrCreatorAlreadyExists is the sentinel raised by CreatorRepo.Create when
// a 23505 fires on the creators_iin_unique constraint — a creator with this
// IIN has already been provisioned. User-facing code mapping comes in 18b.
var ErrCreatorAlreadyExists = errors.New("creator with this iin already exists")

// ErrCreatorTelegramAlreadyTaken is the sentinel raised on a 23505 against
// creators_telegram_user_id_unique — the Telegram account is already bound
// to another creator. User-facing code mapping comes in 18b.
var ErrCreatorTelegramAlreadyTaken = errors.New("creator with this telegram_user_id already exists")

// ErrCreatorApplicationNotApprovable is the sentinel raised on a 23505
// against creators_source_application_id_unique — the source application has
// already produced a creator (concurrent approve lost the race). User-facing
// code mapping comes in 18b.
var ErrCreatorApplicationNotApprovable = errors.New("creator application has already been approved")
