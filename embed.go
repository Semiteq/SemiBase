// Package semibase carries repository-level assets embedded into the binary.
package semibase

import _ "embed"

// SemiplotTagsSQL is the DDL for the one object SemiBase adds to the archive database.
//
//go:embed sql/semiplot_tags.sql
var SemiplotTagsSQL string
