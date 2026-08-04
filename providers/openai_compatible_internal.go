// Copyright (C) 2026 Petr Malik
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at <https://mozilla.org/MPL/2.0/>.

package providers

import (
	"github.com/openai/openai-go/v3/packages/respjson"
)

// extractExtraFieldRaw returns the raw JSON string for a non-standard field if it is
// present and non-null. The SDK's respjson.Field.Valid() returns false for ExtraFields,
// so presence is checked via Raw() instead.
func extractExtraFieldRaw(extraFields map[string]respjson.Field, key string) (string, bool) {
	if field, ok := extraFields[key]; ok && field.Raw() != "" && field.Raw() != "null" {
		return field.Raw(), true
	}
	return "", false
}
