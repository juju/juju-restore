// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// This file contains helper functions for generic operations commonly needed
// when implementing an interactive command.

type userAbortedError string

func (e userAbortedError) Error() string {
	return string(e)
}

// IsUserAbortedError returns true if err is of type userAbortedError.
func IsUserAbortedError(err error) bool {
	_, ok := errors.Cause(err).(userAbortedError)
	return ok
}

// UserConfirmYes returns an error if we do not read a "y" or "yes" from user
// input.
func UserConfirmYes(ctx *cmd.Context) error {
	scanner := bufio.NewScanner(ctx.Stdin)
	scanner.Scan()
	err := scanner.Err()
	if err != nil && err != io.EOF {
		return errors.Trace(err)
	}
	answer := strings.ToLower(scanner.Text())
	if answer != "y" && answer != "yes" {
		return errors.Trace(userAbortedError("aborted"))
	}
	return nil
}

// Notify will post message to an io.Writer of the given cmd.Context.
// This ensures that all message that require user attention
// go consistently to the same writer.
func Notify(ctx *cmd.Context, message string) {
	fmt.Fprintf(ctx.Stdout, message)
}
