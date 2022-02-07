// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
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

// NewUserInteractions constructs user interactions with given context.
func NewUserInteractions(ctx *cmd.Context) *UserInteractions {
	return &UserInteractions{
		ctx:     ctx,
		scanner: bufio.NewScanner(ctx.Stdin),
	}
}

// UserInteractions communicates with the user
// by providing feedback and by collecting user input.
type UserInteractions struct {
	ctx     *cmd.Context
	scanner *bufio.Scanner
}

// UserConfirmYes returns an error if we do not read a "y" or "yes" from user
// input.
func (ui *UserInteractions) UserConfirmYes() error {
	for ui.scanner.Scan() {
		s := strings.ToLower(ui.scanner.Text())
		switch s {
		case "y", "yes":
			return nil
		case "n", "no", "":
			return errors.Trace(userAbortedError("aborted"))
		}
		ui.Notify(fmt.Sprintf("Invalid response %q. Please answer (y/N): ", s))
	}
	if ui.scanner.Err() != nil {
		return errors.Trace(ui.scanner.Err())
	}
	return errors.Errorf("no input")
}

// Notify will post message to an io.Writer of the given cmd.Context.
// This ensures that all messages that require user attention
// go consistently to the same writer.
func (ui *UserInteractions) Notify(message string) {
	fmt.Fprintf(ui.ctx.Stdout, message)
}
