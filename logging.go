// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"

	"github.com/juju/ansiterm"
	"github.com/juju/loggo"
	"github.com/juju/loggo/loggocolor"
)

type colorWriter struct {
	writer *ansiterm.Writer
}

// NewColorWriter will write out colored severity levels if the writer is
// outputting to a terminal.
func NewColorWriter(writer io.Writer) loggo.Writer {
	return &colorWriter{ansiterm.NewWriter(writer)}
}

// Write implements Writer.
//   WARNING The message...
func (w *colorWriter) Write(entry loggo.Entry) {
	loggocolor.SeverityColor[entry.Level].Fprintf(w.writer, entry.Level.String())
	fmt.Fprintf(w.writer, " %s\n", entry.Message)
}
