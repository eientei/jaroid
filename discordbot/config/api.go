// Package config with configuration models and utilities
package config

import (
	"io"

	yaml "gopkg.in/yaml.v2"
)

// Read reads configuration
func Read(reader io.Reader) (root *Root, err error) {
	root = &Root{}
	err = yaml.NewDecoder(reader).Decode(root)

	return
}

// Write writes configuration
func Write(writer io.Writer, root *Root) (err error) {
	err = yaml.NewEncoder(writer).Encode(root)

	return
}
