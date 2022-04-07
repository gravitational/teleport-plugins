package config

import "fmt"

// RecipientsMap is a mapping of roles to recipient(s).
type RecipientsMap map[string][]string

func (r *RecipientsMap) UnmarshalTOML(in interface{}) error {
	*r = make(RecipientsMap)

	recipientsMap, ok := in.(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected type for recipients %T", in)
	}

	for k, v := range recipientsMap {
		switch val := v.(type) {
		case string:
			(*r)[k] = []string{val}
		case []interface{}:
			for _, str := range val {
				str, ok := str.(string)
				if !ok {
					return fmt.Errorf("unexpected type for recipients value %T", v)
				}
				(*r)[k] = append((*r)[k], str)
			}
		default:
			return fmt.Errorf("unexpected type for recipients value %T", v)
		}
	}

	return nil
}
