/*
Mistral AI API

Our Chat Completion and Embeddings APIs specification. Create your account on [La Plateforme](https://console.mistral.ai) to get access and read the [docs](https://docs.mistral.ai) to learn how to use it.

API version: 1.0.0
*/

// Manually maintained — protected by .openapi-generator-ignore.
// The generator produces invalid `*OsFile` identifiers from the anyOf(string,
// string+binary) schema. Both variants are strings in a JSON context, so this
// is simplified to a string-based anyOf.

package mistralai

import (
	"encoding/json"
	"fmt"
)

// InputAudio struct for InputAudio.
type InputAudio struct {
	String *string
}

// Unmarshal JSON data into any of the pointers in the struct.
func (dst *InputAudio) UnmarshalJSON(data []byte) error {
	var err error
	// try to unmarshal JSON data into String
	err = json.Unmarshal(data, &dst.String)
	if err == nil {
		jsonString, _ := json.Marshal(dst.String)
		if string(jsonString) == "{}" { // empty struct
			dst.String = nil
		} else {
			return nil // data stored in dst.String, return on the first match
		}
	} else {
		dst.String = nil
	}

	return fmt.Errorf("data failed to match schemas in anyOf(InputAudio)")
}

// Marshal data from the first non-nil pointers in the struct to JSON.
func (src InputAudio) MarshalJSON() ([]byte, error) {
	if src.String != nil {
		return json.Marshal(&src.String)
	}

	return nil, nil // no data in anyOf schemas
}

type NullableInputAudio struct {
	value *InputAudio
	isSet bool
}

func (v NullableInputAudio) Get() *InputAudio {
	return v.value
}

func (v *NullableInputAudio) Set(val *InputAudio) {
	v.value = val
	v.isSet = true
}

func (v NullableInputAudio) IsSet() bool {
	return v.isSet
}

func (v *NullableInputAudio) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableInputAudio(val *InputAudio) *NullableInputAudio {
	return &NullableInputAudio{value: val, isSet: true}
}

func (v NullableInputAudio) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableInputAudio) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
