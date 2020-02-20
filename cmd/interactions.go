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
	"golang.org/x/crypto/ssh/terminal"
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
func NewUserInteractions(ctx *cmd.Context, charFunc func(*cmd.Context) (string, error)) *UserInteractions {
	return &UserInteractions{ctx, charFunc}
}

// UserInteractions communicates with the user
// by providing feedback and by collecting user input.
type UserInteractions struct {
	ctx             *cmd.Context
	readOneCharFunc func(*cmd.Context) (string, error)
}

func ReadOneChar(ctx *cmd.Context) (string, error) {
	// fd 0 is stdin
	state, err := terminal.MakeRaw(0)
	if err != nil {
		logger.Errorf("setting stdin to raw:", err)
		return "", errors.Trace(err)
	}
	defer func() {
		if err := terminal.Restore(0, state); err != nil {
			logger.Warningf("warning, failed to restore terminal:", err)
		}
	}()
	in := bufio.NewReader(ctx.Stdin)
	r, _, err := in.ReadRune()
	if err != nil {
		return "", errors.Trace(err)
	}
	// Because we are in raw mode, user response is not visible.
	// Display user response explicitly to avoid confusion.
	fmt.Fprintf(ctx.Stdout, fmt.Sprintf("%v\r\n", string(r)))
	return string(r), nil
}

// UserConfirmYes returns an error if we do not read a "y" or "yes" from user
// input.
func (ui *UserInteractions) UserConfirmYes() error {
	prompt := func() (bool, error) {
		answer, err := ui.readOneCharFunc(ui.ctx)
		if err != nil {
			return false, errors.Trace(err)
		}
		lowCased := strings.ToLower(answer)
		if lowCased == "y" {
			return true, nil
		}
		if lowCased == "n" || lowCased == "\n" || lowCased == "\r" {
			return true, errors.Trace(userAbortedError("aborted"))
		}
		ui.Notify(fmt.Sprintf("Invalid answer %q. Please answer (y/N) or Enter to default to N: ", answer))
		return false, nil
	}

	for {
		proceed, err := prompt()
		if err != nil {
			return errors.Trace(err)
		}
		if proceed {
			return nil
		}
	}
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
