package utils

import (
	"encoding/json"
	"fmt"
)

func ConvertParams(params interface{}, target interface{}) error {
	if paramsArray, ok := params.([]interface{}); ok {
		if len(paramsArray) != 1 {
			return fmt.Errorf("params must be a single-item array")
		}

		paramsJson, err := json.Marshal(paramsArray[0])
		if err != nil {
			return err
		}

		return json.Unmarshal(paramsJson, target)
	}

	// If params is not an array, assume it's a map (single object)
	if paramsMap, ok := params.(map[string]interface{}); ok {
		paramsJson, err := json.Marshal(paramsMap)
		if err != nil {
			return err
		}

		return json.Unmarshal(paramsJson, target)
	}

	return fmt.Errorf("params should be either a single-item array or a map")
}
