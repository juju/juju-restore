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

// NewUserInteractions constructs user interactions with given context.
func NewUserInteractions(ctx *cmd.Context) *UserInteractions {
	return &UserInteractions{ctx}
}

// UserInteractions communicates with the user
// by providing feedback and by collecting user input.
type UserInteractions struct {
	ctx *cmd.Context
}

// UserConfirmYes returns an error if we do not read a "y" or "yes" from user
// input.
func (ui *UserInteractions) UserConfirmYes() error {
	scanner := bufio.NewScanner(byteAtATimeReader{ui.ctx.Stdin})
	done := !scanner.Scan()
	if done {
		if err := scanner.Err(); err != nil {
			return errors.Trace(err)
		}
	}

	answer := strings.ToLower(scanner.Text())
	if done && answer == "" {
		return io.EOF
	}
	if answer != "y" {
		return errors.Trace(userAbortedError("aborted"))
	}
	return nil
}

// Notify will post message to an io.Writer of the given cmd.Context.
// This ensures that all messages that require user attention
// go consistently to the same writer.
func (ui *UserInteractions) Notify(message string) {
	fmt.Fprintf(ui.ctx.Stdout, message)
}

// byteAtATimeReader causes all reads to return a single byte.  This prevents
// things line bufio.scanner from reading past the end of a line, which can
// cause problems when we do wacky things like reading directly from the
// terminal for password style prompts.
type byteAtATimeReader struct {
	io.Reader
}

func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}
