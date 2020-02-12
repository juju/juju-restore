// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package core

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// NewUnhealthyMembersError returns an error reporting the status of
// the given members.
func NewUnhealthyMembersError(members []ReplicaSetMember) error {
	return &unhealthyMembersError{members: members}
}

type unhealthyMembersError struct {
	members []ReplicaSetMember
}

// Error is part of error.
func (e *unhealthyMembersError) Error() string {
	var parts []string
	for _, m := range e.members {
		parts = append(parts, m.String())
	}
	return fmt.Sprintf("unhealthy replica set members: %s", strings.Join(parts, ", "))
}

// IsUnhealthyMembersError returns whether the cause of this error is
// that replica set members are unhealthy.
func IsUnhealthyMembersError(err error) bool {
	_, ok := errors.Cause(err).(*unhealthyMembersError)
	return ok
}
