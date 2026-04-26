package pi

import _ "embed"

const entireCmdPlaceholder = "__ENTIRE_CMD__"

//go:embed entire_extension.ts
var extensionTemplate string
