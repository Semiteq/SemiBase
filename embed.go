package semibase

import _ "embed"

//go:embed sql/semiplot_tags.sql
var SemiplotTagsSQL string

//go:embed sql/semiplot_register.sql
var SemiplotRegisterSQL string

//go:embed sql/semiplot_groups.sql
var SemiplotGroupsSQL string

//go:embed sql/semiplot_meta.sql
var SemiplotMetaSQL string

//go:embed sql/trends.sql
var TrendsSQL string
