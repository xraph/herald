package driver

import "strings"

// DataPayload returns the provider settings meant for the recipient's
// payload: keys starting with "data.", with the prefix removed. Nothing else
// in the merged credentials-and-settings map (API keys, tokens, from
// addresses) is ever forwarded. It returns nil when there are none.
func DataPayload(data map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range data {
		if name, ok := strings.CutPrefix(k, "data."); ok && name != "" {
			out[name] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
