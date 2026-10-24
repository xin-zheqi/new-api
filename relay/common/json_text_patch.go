package common

import "strings"

type jsonTextPatch struct {
	start, end int
	value      string
}

// Patches arrive in array order. Rebuild once, instead of copying a potentially
// large conversation for each repaired item; unchanged JSON stays verbatim.
func applyJSONTextPatches(data string, patches []jsonTextPatch) string {
	size := len(data)
	for _, patch := range patches {
		size += len(patch.value) - (patch.end - patch.start)
	}
	var out strings.Builder
	out.Grow(size)
	last := 0
	for _, patch := range patches {
		out.WriteString(data[last:patch.start])
		out.WriteString(patch.value)
		last = patch.end
	}
	out.WriteString(data[last:])
	return out.String()
}
