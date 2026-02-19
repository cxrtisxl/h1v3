package registry

func getString(params map[string]any, key string) string {
	v, _ := params[key].(string)
	return v
}

func getStringSlice(params map[string]any, key string) []string {
	raw, ok := params[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}
