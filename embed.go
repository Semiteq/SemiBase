package semibase

import _ "embed"

//go:embed sql/semiplot_tags.sql
var SemiplotTagsSQL string

//go:embed sql/trends.sql
var TrendsSQL string
