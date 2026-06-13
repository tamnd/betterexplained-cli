package cli

import (
	"errors"

	"github.com/tamnd/betterexplained-cli/betterexplained"
)

func isNotFound(err error) bool {
	return errors.Is(err, betterexplained.ErrNotFound)
}
