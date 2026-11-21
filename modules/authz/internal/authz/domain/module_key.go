package domain

import (
	"errors"
	"regexp"
	"strings"
)

var ErrInvalidModuleKey = errors.New("invalid module key")

type ModuleKey string

var moduleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func NewModuleKey(raw string) (ModuleKey, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if !moduleKeyPattern.MatchString(value) {
		return "", ErrInvalidModuleKey
	}
	return ModuleKey(value), nil
}

func (k ModuleKey) String() string {
	return string(k)
}
